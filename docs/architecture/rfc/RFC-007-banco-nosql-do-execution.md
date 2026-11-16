# RFC-007 — Banco NoSQL do Execution Service

| | |
|---|---|
| **Status** | Aceita |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |
| **Relacionado** | [database.md](../database.md), [RFC-002](./RFC-002-escolha-do-banco-de-dados.md), [ADR-009](../adr/ADR-009-microsservicos-e-banco-por-servico.md) |

## Resumo

Usar **Amazon DynamoDB** com cobrança por requisição como banco do Execution Service (ordens de trabalho, catálogo, estoque, outbox e mensagens processadas).

## Contexto / Problema

A Fase 4 pede ao menos um banco relacional e um NoSQL entre os microsserviços. O Execution Service é o candidato natural ao NoSQL: a ordem de trabalho é um agregado autocontido, lido e gravado inteiro pelo id da ordem de serviço, com os serviços e as peças do diagnóstico embutidos. Não há junções. O que ele exige do banco é **atomicidade na reserva de estoque**: debitar várias peças, mudar o estado da ordem e registrar a resposta da saga, tudo ou nada.

## Opções consideradas

| Critério | DynamoDB | DocumentDB | MongoDB Atlas | MongoDB no cluster |
|---|---|---|---|---|
| Modelo | Chave-valor / documento | Documento (API MongoDB) | Documento | Documento |
| Transação multi-item | `TransactWriteItems`, até 100 itens, com condição | Sim | Sim | Sim (replica set) |
| Custo parado | Zero | Instância mínima ≈ US$ 0,08/h + armazenamento | M0 gratuito, fora da VPC | Consome nós e volume |
| Provisionamento | Segundos | 10 a 15 min | Fora do Terraform da AWS | Helm + volume persistente |
| Acesso | IAM por tabela, sem senha (IRSA) | Usuário e senha, dentro da VPC | Usuário e senha pela internet | Usuário e senha |
| Operação | Nenhuma | Patches e backup gerenciados | Gerenciado por terceiro | Backup e upgrade por conta própria |
| Emulador local | DynamoDB Local | MongoDB em container | MongoDB em container | o próprio |
| Consultas ad hoc | Limitadas (chave, índices, varredura) | Ricas | Ricas | Ricas |

## Proposta

1. **Quatro tabelas**, chave de partição `id` (string), `PAY_PER_REQUEST`, criptografia em repouso, criadas pelo Terraform do Execution Service:
   - `oficina-execution-work-orders` — `id` é o id da ordem de serviço; itens do diagnóstico embutidos.
   - `oficina-execution-catalog` — peças (com saldo de estoque) e serviços.
   - `oficina-execution-outbox` — respostas da saga a publicar.
   - `oficina-execution-processed-messages` — mensagens já consumidas.
2. **Reserva e devolução de estoque com `TransactWriteItems`**: a atualização de cada peça leva a condição de saldo suficiente; se uma falha, nada é gravado e o serviço responde `ExecutionRejected`.
3. **Outbox na mesma transação** da mudança de estado.
4. **Listagens por varredura**: a fila de ordens de trabalho e o catálogo são pequenos; a listagem por status filtra na leitura.
5. **Local e testes**: DynamoDB Local, com `DYNAMODB_CREATE_TABLES=true` criando as tabelas no startup.

## Consequências

- **Positivas:** nenhuma instância ligada (custo zero parado e centavos em uso); provisionamento e destruição em segundos; acesso por IAM com a role do pod; a transação condicional resolve a concorrência na reserva de estoque sem lock de aplicação.
- **Negativas:** consultas limitadas — métricas e listagens fazem varredura, aceitável no volume de uma oficina e a rever com um índice secundário por status se crescer; limite de 100 itens por transação (uma ordem com mais de ~95 peças distintas não caberia); sem esquema nem migrations, a evolução do item é responsabilidade do código; acoplamento ao SDK da AWS, contido nos repositórios da camada de infraestrutura.
- **Operação:** recuperação point-in-time disponível por variável do Terraform, desligada no ambiente de demonstração.

## Perguntas abertas

- Índice secundário global por `status` quando a fila de ordens de trabalho crescer.
- TTL na tabela de mensagens processadas para não crescer indefinidamente.
