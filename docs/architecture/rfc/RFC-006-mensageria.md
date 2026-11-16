# RFC-006 — Mensageria entre os microsserviços

| | |
|---|---|
| **Status** | Aceita |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |
| **Relacionado** | [messaging-contracts.md](../messaging-contracts.md), [ADR-010](../adr/ADR-010-saga-orquestrada.md), [ADR-011](../adr/ADR-011-outbox-transacional-e-consumo-idempotente.md) |

## Resumo

Usar **Amazon SQS** (filas padrão, uma por consumidor, cada uma com DLQ) para os comandos e as respostas da saga da ordem de serviço.

## Contexto / Problema

A Fase 4 divide o monólito em três microsserviços e exige comunicação assíncrona e uma saga com compensação. O orquestrador (OS Service) precisa enviar comandos ao Billing e ao Execution e receber as respostas, sem perder mensagens quando um serviço estiver fora do ar e aceitando passos que demoram dias (aprovação de orçamento, pagamento). O ambiente é criado e destruído a cada sessão de testes, então o custo com tudo parado e o tempo de provisionamento pesam.

## Opções consideradas

| Critério | Amazon SQS | RabbitMQ (Amazon MQ ou no cluster) | Kafka (MSK) | SNS + SQS (fan-out) |
|---|---|---|---|---|
| Operação | Gerenciado, sem broker | Broker para manter (instância ou pods + volume) | Cluster de brokers | Gerenciado |
| Custo parado | Zero | Amazon MQ `mq.t3.micro` ≈ US$ 0,03/h; no cluster consome nós | MSK a partir de ≈ US$ 0,21/h por broker | Zero |
| Custo em uso | US$ 0,40 por milhão; 1 milhão/mês grátis | Fixo | Fixo | Semelhante ao SQS |
| Provisionamento | Segundos, Terraform | Minutos; Helm ou Amazon MQ | 20 a 30 min | Segundos |
| Fila de mensagens com falha | Nativa (`redrive_policy`) | Dead-letter exchange, configurado à mão | Tópico de retry por convenção | Nativa nas filas |
| Controle de acesso | IAM por fila, sem senha (IRSA) | Usuário e senha no broker | ACL ou IAM (MSK) | IAM |
| Ordenação | Sem ordem nas filas padrão | Por fila | Por partição | Sem ordem |
| Roteamento | Uma fila por destino | Exchanges e bindings | Tópicos e grupos | Assinaturas com filtro |
| Aderência ao problema | Comando ponto a ponto com resposta: encaixe direto | Atende, com mais operação | Pensado para fluxo de eventos e replay; sobra | Útil para um evento com vários interessados; aqui cada mensagem tem um destinatário |

## Proposta

1. **Três filas padrão**, cada uma do serviço que a consome, criada pelo Terraform do próprio repositório:
   - `oficina-execution-commands` (Execution)
   - `oficina-billing-commands` (Billing)
   - `oficina-saga-replies` (OS)
2. **Uma DLQ por fila** (`-dlq`), com `maxReceiveCount = 5` e retenção de 14 dias.
3. **Long polling** de 20 s e visibilidade de 30 s.
4. **Acesso por IAM**: a role do pod de cada serviço consome só a própria fila e publica só nas filas de destino ([ADR-013](../adr/ADR-013-infraestrutura-por-servico-e-irsa.md)).
5. **Ordem garantida pelo fluxo, não pela fila**: o orquestrador só envia o próximo comando depois da resposta do anterior, e o status da ordem só avança. Filas FIFO não são necessárias.
6. **Envelope único** em JSON com `message_id`, `type`, `saga_id`, `order_id`, `occurred_at` e `payload`; contexto de trace W3C nos atributos da mensagem.
7. **Local**: ElasticMQ emula o SQS, sem conta AWS.

## Consequências

- **Positivas:** nenhum broker para operar, custo zero com o ambiente parado e desprezível em uso; DLQ e criptografia nativas; acesso sem senha; criação e destruição em segundos junto com o resto do serviço.
- **Negativas:** entrega ao menos uma vez e sem ordenação, o que obriga consumo idempotente e tolerância a respostas fora de ordem ([ADR-011](../adr/ADR-011-outbox-transacional-e-consumo-idempotente.md)); acoplamento ao SDK da AWS, contido atrás das portas `PublisherService` e do consumidor na camada de infraestrutura; sem replay de histórico como no Kafka.
- **Operação:** alerta sobre mensagens nas DLQs e sobre o evento `message.failed`.

## Perguntas abertas

- Se surgirem eventos com vários consumidores (por exemplo, relatórios), acrescentar um tópico SNS na frente das filas sem alterar os consumidores atuais.
