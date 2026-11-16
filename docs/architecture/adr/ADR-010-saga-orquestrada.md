# ADR-010 — Saga orquestrada no OS Service

| | |
|---|---|
| **Status** | Aceita |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |
| **Relacionado** | [sequence-service-order.md](../sequence-service-order.md), [messaging-contracts.md](../messaging-contracts.md), [ADR-009](./ADR-009-microsservicos-e-banco-por-servico.md), [ADR-011](./ADR-011-outbox-transacional-e-consumo-idempotente.md) |

## Contexto

O fluxo da ordem de serviço atravessa três bancos de dados (OS, Billing, Execution) e um serviço externo de pagamento. Ele não cabe em uma transação e pode ficar dias parado esperando o cliente aprovar o orçamento ou pagar. Quando um passo falha depois de outros já terem sido concluídos, o que foi feito precisa ser desfeito em uma ordem definida: estornar o pagamento antes de cancelar o orçamento, cancelar o orçamento antes de liberar a ordem de trabalho.

O padrão Saga resolve isso com transações locais e compensações. Faltava decidir a forma de coordenação.

## Decisão

Adotar uma **saga orquestrada**, com o orquestrador dentro do OS Service.

| Critério | Orquestrada (escolhida) | Coreografada |
|---|---|---|
| Onde está a regra do fluxo | Em um lugar só: o orquestrador | Espalhada pelos consumidores de eventos de cada serviço |
| Ordem das compensações | Explícita; o orquestrador envia uma por vez | Implícita; cada serviço precisa saber a quem reagir |
| Acompanhamento | O estado da saga e seu histórico ficam em uma tabela | É preciso reconstruir o estado a partir dos eventos de todos os serviços |
| Acoplamento | Os participantes só conhecem seus comandos e respostas | Os serviços conhecem os eventos uns dos outros |
| Risco | O orquestrador concentra a lógica | Ciclos de eventos difíceis de enxergar |

Como funciona:

- **Passos:** `diagnosis` → `budget_approval` → `payment` → `execution`. O orquestrador envia um comando, espera a resposta e só então envia o próximo.
- **Estados:** `running` → `completed`, ou `running` → `compensating` → `compensated`.
- **Compensações** enviadas uma por vez, na ordem inversa; a próxima só sai com a confirmação da anterior.

| Falha | Compensações, em ordem |
|---|---|
| `DiagnosisFailed` | nenhuma |
| `BudgetRejected` | `CancelWorkOrder` |
| `PaymentFailed` | `CancelBudget` → `CancelWorkOrder` |
| `ExecutionRejected` | `RefundPayment` → `CancelBudget` → `CancelWorkOrder` |
| Cancelamento pelo operador ou prazo esgotado | conforme o passo em que a saga está |

- **Persistência:** a saga fica nas tabelas `sagas` e `saga_steps` do PostgreSQL do OS, gravadas na **mesma transação** da ordem de serviço e do outbox. O status que o cliente consulta e o estado da saga nunca divergem.
- **Sem estado em memória:** qualquer réplica continua uma saga a partir do banco; o controle otimista de versão impede duas réplicas de aplicar a mesma resposta.
- **Prazo por passo** (`SAGA_STEP_TIMEOUT`, padrão 72 h): um monitor cancela e compensa as sagas vencidas.
- **Consulta:** `GET /api/v1/service-orders/{id}/saga` devolve o estado e o registro de comandos, respostas, falhas e compensações.

## Por que orquestrada

O fluxo é **linear, com quatro passos e uma ordem estrita de compensação**. A ordem de serviço já é o agregado que representa o processo para o cliente, então o OS Service é o lugar natural do orquestrador. Com coreografia, a ordem das compensações ficaria implícita em três serviços e qualquer mudança no fluxo exigiria alterar todos eles; responder "em que ponto está esta ordem e por que parou" exigiria correlacionar eventos de três serviços.

## Consequências

**Positivas**
- A regra do fluxo e das compensações está em dois arquivos (`orchestrator.go` e `transitions.go`) e é testada isoladamente, sem fila nem banco.
- Os participantes ficam simples: recebem um comando, fazem uma transação local e respondem.
- O histórico da saga serve de trilha de auditoria e de ferramenta de suporte.

**Negativas**
- O OS Service concentra o conhecimento do fluxo: incluir um passo exige alterar o orquestrador.
- Se o OS Service estiver fora do ar, as sagas não avançam. Os participantes continuam atendendo e as filas guardam as respostas até ele voltar.
- Há uma troca de mensagens a mais por passo (comando e resposta) em comparação com eventos encadeados.

## Alternativas rejeitadas

- **Saga coreografada** — pelos motivos da tabela acima.
- **Transação distribuída (2PC)** — o Mercado Pago e o DynamoDB não participam de two-phase commit, e o fluxo dura dias.
- **Motor de workflow externo (Step Functions, Temporal)** — resolveria o estado e os prazos, mas tiraria a regra do domínio do código do serviço e acrescentaria uma dependência a provisionar; o fluxo é pequeno o bastante para uma tabela e uma máquina de estados.
