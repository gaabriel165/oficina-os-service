# ADR-011 — Outbox transacional e consumo idempotente

| | |
|---|---|
| **Status** | Aceita |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |
| **Relacionado** | [RFC-006](../rfc/RFC-006-mensageria.md), [ADR-010](./ADR-010-saga-orquestrada.md), [messaging-contracts.md](../messaging-contracts.md) |

## Contexto

Cada passo da saga faz duas coisas: muda o estado no banco do serviço e publica uma mensagem no SQS. Banco e fila não compartilham transação. Se o serviço gravar e cair antes de publicar, a saga para; se publicar e a gravação falhar, o outro serviço age sobre algo que não aconteceu. Além disso, o SQS entrega ao menos uma vez e sem ordem: a mesma mensagem pode chegar duas vezes e uma resposta pode chegar depois da seguinte.

## Decisão

**Publicação — outbox transacional**

- A mensagem de saída é gravada em uma tabela de outbox **na mesma transação** da mudança de estado.
  - OS e Billing: tabela `outbox_messages` no PostgreSQL.
  - Execution: tabela `oficina-execution-outbox` no DynamoDB, na mesma `TransactWriteItems`.
- Um **relay** em cada serviço lê as pendentes em intervalos curtos, publica no SQS e marca como publicada (PostgreSQL) ou apaga (DynamoDB).
- No PostgreSQL a leitura usa `FOR UPDATE SKIP LOCKED`, para que várias réplicas dividam o trabalho sem publicar a mesma linha ao mesmo tempo.
- O contexto de trace é guardado junto da mensagem, para que o trace continue mesmo com a publicação acontecendo depois.

**Consumo — idempotência em duas camadas**

- Cada serviço registra o `message_id` das mensagens processadas (`processed_messages`) e ignora repetições, registrando `message.duplicate`.
- Os casos de uso conferem o estado antes de agir: `GenerateBudget` para uma ordem que já tem orçamento devolve o existente; `StartExecution` para uma ordem já em execução não reserva de novo; as compensações respondem a confirmação mesmo quando não há mais nada a desfazer.

**Ordem e concorrência**

- O status da ordem de serviço só avança; uma resposta atrasada de um passo anterior é registrada e ignorada (`saga.reply_ignored`).
- A saga tem controle otimista de versão (`UPDATE ... WHERE version = n`). Em conflito a mensagem não é confirmada e volta à fila.

**Falha**

- Uma mensagem cujo processamento falha não é apagada; volta após o tempo de visibilidade e, na quinta entrega, vai para a DLQ.

## Consequências

**Positivas**
- Nenhuma mensagem é perdida entre gravar e publicar, e nenhuma é publicada sem a mudança correspondente.
- Entrega duplicada e fora de ordem deixam de ser um problema de correção.
- O outbox é uma trilha do que cada serviço decidiu publicar.

**Negativas**
- A publicação tem um atraso igual ao intervalo do relay.
- A garantia é "ao menos uma vez": o relay pode publicar e cair antes de marcar, repetindo a mensagem — por isso a idempotência no consumidor é obrigatória, não opcional.
- Duas tabelas a mais por serviço, que crescem com o tempo e pedem limpeza periódica em um ambiente permanente.
- No DynamoDB o relay faz varredura da tabela de outbox; barato enquanto ela fica quase vazia.

## Alternativas rejeitadas

- **Publicar direto no SQS depois do commit** — perde a mensagem se o processo cair entre os dois passos.
- **Publicar antes do commit** — anuncia mudanças que podem não ser gravadas.
- **Captura de mudanças (CDC, DynamoDB Streams, Debezium)** — elimina o relay, mas acrescenta infraestrutura e um mecanismo diferente em cada banco.
- **Filas FIFO com deduplicação do SQS** — a janela de deduplicação é de 5 minutos e não cobre reentregas tardias; não dispensa a idempotência no consumidor.
