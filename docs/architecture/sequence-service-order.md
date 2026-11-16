# Diagrama de Sequência — Saga da Ordem de Serviço

| | |
|---|---|
| **Status** | Vigente |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |
| **Relacionado** | [messaging-contracts.md](./messaging-contracts.md), [ADR-010](./adr/ADR-010-saga-orquestrada.md), [ADR-011](./adr/ADR-011-outbox-transacional-e-consumo-idempotente.md) |

Desde a Fase 4 a ordem de serviço é conduzida por uma **saga orquestrada**. O OS Service é o orquestrador: envia comandos ao Execution Service e ao Billing Service por filas SQS e reage às respostas. Cada serviço grava no próprio banco; nenhuma transação atravessa serviços.

## 1. Caminho feliz

```mermaid
sequenceDiagram
    autonumber
    actor Op as Operador / mecânico
    actor Cl as Cliente
    participant OS as OS Service (orquestrador)
    participant QE as SQS execution-commands
    participant Ex as Execution Service
    participant QR as SQS saga-replies
    participant QB as SQS billing-commands
    participant Bi as Billing Service
    participant MP as Mercado Pago

    Op->>OS: POST /api/v1/service-orders { customer_id, vehicle_id, notes }
    OS->>OS: transação: INSERT service_orders, sagas, outbox (RequestDiagnosis)
    OS-->>Op: 201 { status: received }
    OS->>QE: RequestDiagnosis
    QE->>Ex: abre a ordem de trabalho (awaiting_diagnosis)

    Op->>Ex: PATCH /work-orders/{id}/start-diagnosis
    Ex->>QR: DiagnosisStarted
    QR->>OS: OS → in_diagnosis
    Op->>Ex: POST /work-orders/{id}/services e /parts
    Op->>Ex: PATCH /work-orders/{id}/complete-diagnosis
    Ex->>QR: DiagnosisCompleted { services[], parts[] }
    QR->>OS: saga → passo budget_approval
    OS->>QB: GenerateBudget { cliente, itens a preço congelado }
    QB->>Bi: cria o orçamento e envia e-mail
    Bi->>QR: BudgetGenerated { budget_id, total_amount }
    QR->>OS: OS → waiting_approval

    Cl->>Bi: PATCH /budgets/{id}/approve
    Bi->>QR: BudgetApproved
    QR->>OS: saga → passo payment
    OS->>QB: RequestPayment
    QB->>Bi: cria o pagamento
    Bi->>MP: POST /checkout/preferences
    MP-->>Bi: init_point
    Bi->>QR: PaymentRequested { payment_id, checkout_url }
    QR->>OS: OS → waiting_payment

    Cl->>MP: paga no checkout
    MP->>Bi: POST /api/v1/webhooks/mercadopago (x-signature)
    Bi->>MP: GET /v1/payments/{id}
    Bi->>QR: PaymentConfirmed
    QR->>OS: saga → passo execution
    OS->>QE: StartExecution
    QE->>Ex: reserva o estoque (TransactWriteItems)
    Ex->>QR: ExecutionStarted
    QR->>OS: OS → in_execution

    Op->>Ex: PATCH /work-orders/{id}/finish-execution
    Ex->>QR: ExecutionFinished
    QR->>OS: OS → finished, saga → completed

    Op->>OS: PATCH /api/v1/service-orders/{id}/deliver
    OS-->>Op: 200 { status: delivered }
```

A cada mudança de status o OS Service envia um e-mail ao cliente (best-effort) e grava uma linha em `service_order_status_history`.

## 2. Compensação — estoque insuficiente depois do pagamento

É o caso com mais passos a desfazer: o orçamento foi aprovado e o pagamento confirmado, mas o Execution Service não consegue reservar as peças. O orquestrador cancela a ordem de serviço e envia as compensações **uma por vez, na ordem inversa**; a próxima só sai depois da confirmação da anterior.

```mermaid
sequenceDiagram
    autonumber
    participant OS as OS Service (orquestrador)
    participant Ex as Execution Service
    participant Bi as Billing Service
    participant MP as Mercado Pago
    actor Cl as Cliente

    Note over OS,Bi: Pagamento confirmado, saga no passo execution
    OS->>Ex: StartExecution
    Ex->>Ex: débito condicional falha: estoque insuficiente
    Ex-->>OS: ExecutionRejected { reason }
    OS->>OS: transação: OS → cancelled, saga → compensating,<br/>pendentes = [RefundPayment, CancelBudget, CancelWorkOrder]
    OS-->>Cl: e-mail: ordem cancelada

    OS->>Bi: RefundPayment
    Bi->>MP: POST /v1/payments/{id}/refunds (X-Idempotency-Key)
    Bi-->>OS: PaymentRefunded
    OS->>Bi: CancelBudget
    Bi-->>OS: BudgetCancelled
    OS->>Ex: CancelWorkOrder
    Ex->>Ex: cancela a ordem de trabalho (sem estoque a devolver)
    Ex-->>OS: WorkOrderCancelled
    OS->>OS: saga → compensated
```

### Compensações por tipo de falha

| Falha | Vem de | Compensações, em ordem |
|---|---|---|
| `DiagnosisFailed` | Execution | nenhuma |
| `BudgetRejected` | Billing | `CancelWorkOrder` |
| `PaymentFailed` | Billing | `CancelBudget` → `CancelWorkOrder` |
| `ExecutionRejected` | Execution | `RefundPayment` → `CancelBudget` → `CancelWorkOrder` |
| Cancelamento pelo operador (`PATCH /service-orders/{id}/cancel`) ou prazo esgotado | OS | passo `diagnosis`: `CancelWorkOrder`; passo `budget_approval`: `CancelBudget` → `CancelWorkOrder`; passos `payment` e `execution`: `RefundPayment` → `CancelBudget` → `CancelWorkOrder` |

Os comandos de compensação são idempotentes e toleram ausência do recurso: `RefundPayment` sem pagamento confirmado, `CancelBudget` sem orçamento e `CancelWorkOrder` sem ordem de trabalho apenas respondem a confirmação.

## 3. Estados

```mermaid
stateDiagram-v2
    direction LR
    state "Ordem de serviço" as OS {
        [*] --> received
        received --> in_diagnosis
        in_diagnosis --> waiting_approval
        waiting_approval --> waiting_payment
        waiting_payment --> in_execution
        in_execution --> finished
        finished --> delivered
        received --> cancelled
        in_diagnosis --> cancelled
        waiting_approval --> cancelled
        waiting_payment --> cancelled
        in_execution --> cancelled
        delivered --> [*]
        cancelled --> [*]
    }
```

```mermaid
stateDiagram-v2
    direction LR
    state "Saga" as SG {
        [*] --> running
        running --> completed: ExecutionFinished
        running --> compensating: falha com algo a desfazer
        running --> compensated: falha sem nada a desfazer
        compensating --> compensated: última compensação confirmada
        completed --> [*]
        compensated --> [*]
    }
```

Passos da saga enquanto está `running`: `diagnosis` → `budget_approval` → `payment` → `execution`. O status da ordem só avança: uma resposta atrasada de um passo anterior não faz a ordem voltar.

## 4. Como as garantias entram no fluxo

```mermaid
sequenceDiagram
    autonumber
    participant C as Consumidor SQS
    participant PM as processed_messages
    participant O as Orquestrador
    participant DB as PostgreSQL (OS)
    participant R as Relay do outbox
    participant Q as SQS

    C->>PM: message_id já processado?
    alt repetida
        PM-->>C: sim
        C->>C: log message.duplicate, apaga da fila
    else nova
        C->>O: Handle(reply)
        O->>DB: carrega saga e ordem
        O->>O: transição válida para o estado atual?
        alt resposta fora de ordem ou saga encerrada
            O->>O: log saga.reply_ignored
        else aplicada
            O->>DB: transação: UPDATE service_orders, sagas (WHERE version = n),<br/>INSERT saga_steps, status_history, outbox_messages
        end
        C->>PM: registra message_id, apaga da fila
    end

    loop a cada intervalo
        R->>DB: SELECT ... FROM outbox_messages WHERE published_at IS NULL FOR UPDATE SKIP LOCKED
        R->>Q: SendMessage (atributos traceparent, message_type)
        R->>DB: UPDATE published_at
    end
```

| Garantia | Mecanismo |
|---|---|
| Nenhum comando perdido | Outbox transacional: a mensagem é gravada com a mudança de estado e publicada depois pelo relay |
| Sem efeito duplicado | `processed_messages` no consumidor e verificação de estado nos casos de uso |
| Sem duas réplicas aplicando a mesma resposta | Controle otimista: `UPDATE sagas ... WHERE version = n`; em conflito a mensagem volta à fila |
| Saga parada não fica presa | `deadline` por passo (`SAGA_STEP_TIMEOUT`, padrão 72 h); o monitor cancela e compensa as vencidas |
| Mensagem que sempre falha | Depois de 5 entregas vai para a fila `-dlq` |
| Um trace para o fluxo todo | O contexto W3C é guardado no outbox e viaja nos atributos da mensagem |

## 5. NRQL de referência

| Pergunta | Consulta |
|---|---|
| Volume diário de ordens abertas | `SELECT count(*) FROM Log WHERE event = 'saga.started' TIMESERIES 1 day` |
| Tempo médio em cada status | `SELECT average(numeric(duration_minutes)) FROM Log WHERE event = 'service_order.status_changed' FACET from_status` |
| Sagas concluídas x compensadas | `SELECT count(*) FROM Log WHERE event IN ('saga.completed', 'saga.compensated') FACET event TIMESERIES` |
| Motivos de compensação | `SELECT count(*) FROM Log WHERE event = 'saga.compensating' FACET failure_reason` |
| Avanço da saga por resposta recebida | `SELECT count(*) FROM Log WHERE event = 'saga.step_completed' FACET reply TIMESERIES` |
| Respostas repetidas ou atrasadas | `SELECT count(*) FROM Log WHERE event IN ('saga.reply_ignored', 'message.duplicate') FACET event` |
| Mensagens processadas por serviço | `SELECT count(*) FROM Log WHERE event = 'message.processed' FACET service.name, message_type` |
| Falhas de processamento (alerta) | `SELECT count(*) FROM Log WHERE event = 'message.failed' FACET service.name` |
| Sagas canceladas por prazo (alerta) | `SELECT sum(sagas) FROM Log WHERE event = 'saga.timed_out'` |
| Latência por rota e por serviço | `SELECT percentile(duration.ms, 50, 95) FROM Span WHERE service.name LIKE 'oficina-%' FACET service.name, http.route` |
| Um fluxo de ponta a ponta | `SELECT * FROM Log WHERE order_id = '<id>' SINCE 1 day ago` ou o trace distribuído aberto a partir de qualquer linha |

Todas as linhas levam `request_id`, `trace_id` e `span_id`; as da saga levam `saga_id` e `order_id` (ver [ADR-006](./adr/ADR-006-logs-estruturados-com-correlacao.md)).
