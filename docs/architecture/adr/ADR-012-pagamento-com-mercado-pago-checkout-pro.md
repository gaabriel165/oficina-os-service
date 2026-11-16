# ADR-012 — Pagamento com Mercado Pago (Checkout Pro)

| | |
|---|---|
| **Status** | Aceita |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |
| **Relacionado** | [ADR-009](./ADR-009-microsservicos-e-banco-por-servico.md), [ADR-010](./ADR-010-saga-orquestrada.md), [sequence-service-order.md](../sequence-service-order.md) |

## Contexto

A Fase 4 pede a integração com o Mercado Pago para o pagamento do orçamento. O pagamento é um passo da saga: só depois dele o Execution Service reserva o estoque, e ele precisa ser estornado se a execução for rejeitada. O serviço não tem front-end e o sistema precisa continuar rodando localmente e nos testes sem depender da internet nem de credenciais.

## Decisão

- **Checkout Pro.** O Billing Service cria uma **preferência de pagamento** e devolve o link (`init_point`) em que o cliente paga na página do Mercado Pago. Dados de cartão nunca passam pelo sistema.
- **Porta no domínio.** O gateway é a interface `PaymentGatewayService` (`CreateCheckout`, `FindPayment`, `Refund`), com duas implementações na infraestrutura:
  - `MercadoPagoGateway`, usada com `PAYMENT_GATEWAY=mercadopago`;
  - `FakeGateway`, padrão local, cujo link de checkout aponta para `POST /api/v1/payments/{id}/simulate`.
- **Correlação.** A `external_reference` da preferência é o id do pagamento local; os itens da preferência são os itens do orçamento.
- **Resultado por webhook.** A preferência leva a `notification_url` `POST /api/v1/webhooks/mercadopago`. Ao receber a notificação o serviço:
  1. valida o cabeçalho `x-signature` — HMAC-SHA256 do manifesto `id:<data.id>;request-id:<x-request-id>;ts:<ts>;` com a assinatura secreta da aplicação, comparado em tempo constante; sem assinatura válida responde 401;
  2. consulta o pagamento em `GET /v1/payments/{id}` (a notificação traz só o id, e o estado é sempre lido da origem);
  3. confirma ou reprova o pagamento local e publica `PaymentConfirmed` ou `PaymentFailed` pelo outbox.
- **Respostas do webhook.** Notificação de outro tipo, de pagamento ainda pendente ou de pagamento que não pertence ao serviço recebe 200 e é ignorada; falha ao consultar o Mercado Pago recebe 502, para que ele reenvie.
- **Estorno.** `RefundPayment` chama `POST /v1/payments/{id}/refunds` com `X-Idempotency-Key` derivada do id do pagamento.
- **Segredos.** Access token e assinatura secreta ficam em secrets do GitHub e chegam ao pod pelo `Secret` do Kubernetes; a URL pública do gateway vem do SSM (`/oficina-api/api_gateway_url`).
- **Rota pública.** O webhook é a única rota do Billing fora do Lambda authorizer; a autenticação dele é a assinatura.

## Consequências

**Positivas**
- Sem dados de cartão no sistema e sem tela de pagamento para construir.
- O domínio e os casos de uso não conhecem o Mercado Pago; trocar de provedor é escrever outro adapter.
- O sistema inteiro roda e é testado localmente com o gateway falso; o adapter real é testado contra um servidor HTTP simulado.
- Notificações repetidas são inofensivas: um pagamento já liquidado não muda de estado.

**Negativas**
- A confirmação depende do webhook chegar: exige URL pública (o API Gateway) e, se a notificação se perder além das tentativas do Mercado Pago, o pagamento fica pendente até o prazo do passo da saga cancelar a ordem.
- O cliente sai do fluxo da oficina para pagar na página do Mercado Pago.
- A URL do gateway muda a cada recriação do ambiente; por isso ela é lida do SSM no deploy em vez de ficar fixa no painel do Mercado Pago.
- Estorno é uma chamada externa dentro de uma compensação: se o Mercado Pago estiver indisponível, o comando falha, volta à fila e, depois de cinco tentativas, vai para a DLQ e pede intervenção.

## Alternativas rejeitadas

- **Checkout Transparente / Bricks** — exigiria uma tela própria de pagamento e lidar com tokenização de cartão; fora do escopo de um back-end.
- **Consultar o pagamento por polling** — simples, mas atrasa a confirmação e gera chamadas sem necessidade.
- **Confiar no corpo da notificação sem consultar a API** — a notificação não traz o status e pode ser forjada se a assinatura vazar.
- **SDK oficial em Go** — são três chamadas HTTP; um cliente próprio de ~200 linhas evita a dependência e fica inteiramente coberto por testes.
