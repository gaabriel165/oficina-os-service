# Contratos de Mensagens

| | |
|---|---|
| **Status** | Vigente |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |

Este documento é a fonte única dos contratos trocados entre o **OS Service** (orquestrador da saga), o **Billing Service** e o **Execution Service**. Cada serviço mantém uma cópia dos tipos em `internal/domain/messaging`; qualquer mudança começa aqui.

## 1. Filas

| Fila | Publica | Consome | Conteúdo |
|---|---|---|---|
| `oficina-execution-commands` | OS Service | Execution Service | Comandos da saga para diagnóstico e execução |
| `oficina-billing-commands` | OS Service | Billing Service | Comandos da saga para orçamento e pagamento |
| `oficina-saga-replies` | Billing e Execution | OS Service | Respostas e eventos de progresso |

Cada fila tem uma DLQ com o sufixo `-dlq`. Uma mensagem vai para a DLQ após 5 tentativas de processamento sem sucesso.

## 2. Envelope

Todas as mensagens usam o mesmo envelope JSON no corpo:

```json
{
  "message_id": "5b0c1f0e-3c5e-4c0e-9d57-0d8c8f6a2b11",
  "type": "RequestDiagnosis",
  "saga_id": "0f6f5c5e-8a53-4b0c-b3a2-6c1f6f7d9e21",
  "order_id": "d1e6a7c4-2b1f-4f53-8a37-7d0a3c1b9e55",
  "occurred_at": "2026-11-16T14:03:22Z",
  "payload": {}
}
```

| Campo | Uso |
|---|---|
| `message_id` | Identificador único; chave de idempotência do consumidor |
| `type` | Nome do comando ou da resposta (tabelas abaixo) |
| `saga_id` | Instância da saga; correlaciona comandos e respostas |
| `order_id` | Ordem de serviço à qual a mensagem se refere |
| `occurred_at` | Instante de criação, em UTC (RFC 3339) |
| `payload` | Dados específicos do tipo |

Atributos da mensagem SQS:

| Atributo | Uso |
|---|---|
| `message_type` | Cópia de `type`, para filtro e métricas sem abrir o corpo |
| `traceparent`, `tracestate` | Contexto W3C Trace Context; o consumidor continua o mesmo trace |

## 3. Comandos para o Execution Service

| Tipo | Quando | Payload |
|---|---|---|
| `RequestDiagnosis` | OS aberta | `customer_id`, `vehicle_id`, `notes` |
| `StartExecution` | Pagamento confirmado | — |
| `CancelWorkOrder` | Compensação | `reason` |

## 4. Comandos para o Billing Service

| Tipo | Quando | Payload |
|---|---|---|
| `GenerateBudget` | Diagnóstico concluído | `customer_id`, `customer_name`, `customer_email`, `services[]`, `parts[]` |
| `RequestPayment` | Orçamento aprovado | — |
| `CancelBudget` | Compensação | `reason` |
| `RefundPayment` | Compensação após pagamento confirmado | `reason` |

Itens do orçamento (preço congelado no diagnóstico):

```json
{
  "services": [{ "service_id": "…", "name": "Troca de óleo", "labor_price": 120.0 }],
  "parts": [{ "part_id": "…", "name": "Filtro de óleo", "quantity": 1, "unit_price": 29.9 }]
}
```

## 5. Respostas do Execution Service

| Tipo | Significado | Payload | Efeito na saga |
|---|---|---|---|
| `DiagnosisStarted` | Mecânico iniciou o diagnóstico | `work_order_id` | OS → `in_diagnosis` |
| `DiagnosisCompleted` | Diagnóstico concluído | `work_order_id`, `services[]`, `parts[]` | Envia `GenerateBudget` |
| `DiagnosisFailed` | Diagnóstico não pôde ser feito | `reason` | Cancela a OS |
| `ExecutionStarted` | Estoque reservado, OS na fila de execução | `work_order_id` | OS → `in_execution` |
| `ExecutionRejected` | Estoque insuficiente | `reason` | Compensa: estorno, cancelamentos |
| `ExecutionFinished` | Reparo concluído | `work_order_id` | OS → `finished`, saga concluída |
| `WorkOrderCancelled` | Compensação aplicada (estoque devolvido) | — | Passo de compensação concluído |

## 6. Respostas do Billing Service

| Tipo | Significado | Payload | Efeito na saga |
|---|---|---|---|
| `BudgetGenerated` | Orçamento criado e enviado ao cliente | `budget_id`, `total_amount` | OS → `waiting_approval` |
| `BudgetApproved` | Cliente aprovou | `budget_id` | Envia `RequestPayment` |
| `BudgetRejected` | Cliente recusou ou prazo expirou | `budget_id`, `reason` | Compensa: cancela a ordem de trabalho |
| `PaymentRequested` | Cobrança criada no Mercado Pago | `payment_id`, `checkout_url` | OS → `waiting_payment` |
| `PaymentConfirmed` | Pagamento aprovado | `payment_id` | Envia `StartExecution` |
| `PaymentFailed` | Pagamento recusado ou expirado | `payment_id`, `reason` | Compensa: cancela orçamento e ordem de trabalho |
| `BudgetCancelled` | Compensação aplicada | — | Passo de compensação concluído |
| `PaymentRefunded` | Estorno aplicado | `payment_id` | Passo de compensação concluído |

## 7. Garantias

- **Entrega ao menos uma vez.** Os serviços gravam a mensagem de saída na tabela de outbox, na mesma transação da mudança de estado; um publicador envia ao SQS depois.
- **Consumo idempotente.** O consumidor registra cada `message_id` processado e ignora repetições. Os casos de uso também verificam o estado atual antes de agir.
- **Ordem.** As filas são padrão (sem ordenação). O orquestrador só envia o próximo comando depois de receber a resposta do anterior, então a ordem é garantida pelo fluxo.
- **Falha de processamento.** A mensagem não é removida; volta após o tempo de visibilidade e, na quinta falha, segue para a DLQ, que tem alerta.
