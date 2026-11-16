# Banco de Dados — Justificativa, Modelo Relacional e Ajustes

| | |
|---|---|
| **Status** | Vigente |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |
| **Relacionado** | [RFC-002](./rfc/RFC-002-escolha-do-banco-de-dados.md), [RFC-007](./rfc/RFC-007-banco-nosql-do-execution.md), [ADR-008](./adr/ADR-008-migrations-no-startup-da-aplicacao.md), [ADR-009](./adr/ADR-009-microsservicos-e-banco-por-servico.md) |

> As seções 1 a 5 descrevem o banco único do monólito até a Fase 3 e ficam como histórico. O modelo vigente, com um banco por microsserviço, está na [seção 6](#6-fase-4--um-banco-por-serviço).

## 1. Justificativa formal do PostgreSQL

O domínio da oficina é **transacional e fortemente relacional**: uma ordem de serviço referencia um cliente e um veículo, agrega itens de serviço e peças, e sua aprovação debita estoque. A escolha do PostgreSQL (gerenciado via Amazon RDS) se sustenta nos critérios abaixo.

| Critério | Por que pesa aqui | PostgreSQL | MySQL | DynamoDB | MongoDB |
|---|---|---|---|---|---|
| Transações ACID multi-tabela | Aprovar orçamento = mudar status da OS **e** debitar estoque de N peças, atomicamente | Nativo, MVCC | Nativo (InnoDB) | Limitado (25 itens, custo) | Suportado, mas modelo de documento desincentiva |
| Integridade referencial | OS não pode apontar para cliente/veículo inexistente; veículo pertence a um cliente | FK, `ON DELETE CASCADE` | FK | Não há | Não há |
| Tipos para dinheiro e identificadores | Preços com 2 casas exatas; ids UUID gerados na entidade | `NUMERIC(10,2)`, `UUID` nativo | `DECIMAL`, UUID como `CHAR(36)`/`BINARY(16)` | Number (ponto flutuante) | Decimal128 |
| Consultas analíticas do domínio | Tempo médio de execução por serviço, listagem ordenada por prioridade de status | SQL completo, `AVG`, `EXTRACT(EPOCH ...)`, window functions | SQL completo | Precisa de agregação na aplicação | Aggregation pipeline |
| Ecossistema Go | Migrations versionadas e driver maduro | `golang-migrate`, `pgx`, GORM | idem | SDK AWS | driver oficial |
| Operação gerenciada | Backup, criptografia, patches, Performance Insights sem esforço | RDS | RDS | Serverless | Atlas (fora da AWS) |
| Custo no free/low tier | Ambiente sobe e desce por sessão | `db.t3.micro` gp3 20 GB ≈ US$ 0,02/h | idem | pay-per-request barato, mas modelo não adere | Atlas M0 gratuito, mas fora da VPC |

**Conclusão:** PostgreSQL entrega integridade e transações sem esforço extra na aplicação, tem tipos exatos para dinheiro e UUID, e roda gerenciado dentro da mesma VPC do cluster. Alternativas NoSQL exigiriam reimplementar consistência na camada de aplicação, justamente o que o domínio não tolera.

## 2. Diagrama ER

```mermaid
erDiagram
    users {
        uuid id PK
        varchar email UK
        varchar password_hash
        timestamp created_at
        timestamp updated_at
    }

    customers {
        uuid id PK
        varchar name
        varchar document UK "CPF 11 ou CNPJ 14 digitos"
        varchar document_type "CPF ou CNPJ"
        varchar phone
        varchar email
        varchar status "active ou inactive - Fase 3"
        timestamp created_at
        timestamp updated_at
    }

    vehicles {
        uuid id PK
        uuid customer_id FK
        varchar plate UK "antiga ou Mercosul"
        varchar brand
        varchar model
        int year
        timestamp created_at
        timestamp updated_at
    }

    parts {
        uuid id PK
        varchar name
        text description
        decimal unit_price
        int stock_quantity
        timestamp created_at
        timestamp updated_at
    }

    services {
        uuid id PK
        varchar name
        text description
        decimal labor_price
        int estimated_minutes
        timestamp created_at
        timestamp updated_at
    }

    service_orders {
        uuid id PK
        uuid customer_id FK
        uuid vehicle_id FK
        varchar status "received ... delivered ou cancelled"
        text notes
        decimal total_amount
        timestamp started_at
        timestamp finished_at
        timestamp created_at
        timestamp updated_at
    }

    service_order_items {
        uuid id PK
        uuid service_order_id FK
        uuid service_id FK
        varchar service_name "snapshot"
        decimal labor_price "snapshot"
    }

    service_order_parts {
        uuid id PK
        uuid service_order_id FK
        uuid part_id FK
        varchar part_name "snapshot"
        int quantity
        decimal unit_price "snapshot"
    }

    customers ||--o{ vehicles : "possui"
    customers ||--o{ service_orders : "abre"
    vehicles ||--o{ service_orders : "e atendido em"
    service_orders ||--o{ service_order_items : "contem"
    service_orders ||--o{ service_order_parts : "consome"
    services ||--o{ service_order_items : "e referenciado por"
    parts ||--o{ service_order_parts : "e referenciada por"
```

## 3. Relacionamentos e regras

| Relacionamento | Cardinalidade | Regra de negócio / comportamento no banco |
|---|---|---|
| `customers` → `vehicles` | 1 : N | Um cliente tem vários veículos. `ON DELETE CASCADE`: remover o cliente remove seus veículos. Placa é única no sistema. |
| `customers` → `service_orders` | 1 : N | Toda OS pertence a um cliente identificado por CPF/CNPJ. Sem cascade: OS é histórico e não deve sumir com o cliente. |
| `vehicles` → `service_orders` | 1 : N | A OS é sempre sobre um veículo. O caso de uso valida que `vehicle.customer_id == order.customer_id` antes de gravar. |
| `service_orders` → `service_order_items` | 1 : N | Serviços (mão de obra) incluídos na OS. `ON DELETE CASCADE`: itens não existem sem a OS. |
| `service_orders` → `service_order_parts` | 1 : N | Peças consumidas na OS com quantidade. `ON DELETE CASCADE`. |
| `services` → `service_order_items` | 1 : N | Referência ao catálogo, sem cascade. |
| `parts` → `service_order_parts` | 1 : N | Referência ao estoque, sem cascade. |
| `users` | — | Operadores da oficina. Não se relaciona com clientes: são personas distintas (ver RFC-003). |

### Por que os itens guardam snapshot de nome e preço

`service_order_items.service_name/labor_price` e `service_order_parts.part_name/unit_price` **copiam** os valores do catálogo no momento da inclusão. Assim, reajustar o preço de uma peça amanhã não altera o orçamento já enviado ao cliente hoje. O `total_amount` da OS é calculado pela entidade a partir desses snapshots quando o orçamento é enviado (`SendBudget`), e fica congelado a partir dali.

### Máquina de estados da OS (coluna `status`)

```
received → in_diagnosis → waiting_approval → in_execution → finished → delivered
                                          ↘ cancelled
```

- `started_at` é preenchido na transição para `in_execution`; `finished_at` na transição para `finished`.
- O endpoint de métricas calcula o tempo médio de execução por serviço com `AVG(finished_at - started_at)` sobre OS finalizadas ou entregues.
- A transição é validada **na entidade** (`OrderStatus.CanTransitionTo`), não por trigger ou constraint, mantendo a regra no domínio.

## 4. Ajustes do modelo relacional na Fase 3 — migration `000010`

Nenhuma migration anterior foi alterada; tudo entra em uma nova versão aplicada automaticamente no startup.

### 4.1 Status do cliente

```sql
ALTER TABLE customers ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'active';
```

- Valores: `active` e `inactive`, controlados pelo value object `CustomerStatus` na entidade.
- **Motivação:** o requisito da Lambda de autenticação é *"consultar a existência e o status do cliente na base"*. Um cliente inativo continua existindo (histórico de OS preservado) mas não recebe token.
- A aplicação expõe `PATCH /api/v1/customers/{id}/status` para o operador ativar/inativar, e devolve `status` nas respostas de cliente.

### 4.2 Normalização de `document_type`

```sql
UPDATE customers SET document_type = UPPER(document_type);
```

O seed da Fase 1 gravou `cpf`/`cnpj` em minúsculo enquanto a entidade grava `CPF`/`CNPJ`. A normalização garante que qualquer filtro por tipo de documento (inclusive na Lambda) seja determinístico.

### 4.3 Índices

| Índice | Consulta que atende | Onde a aplicação a executa |
|---|---|---|
| `service_orders(status)` | Listagem de OS ativas (`WHERE status IN (...)`), exclusão de finalizadas/entregues, métricas | `FindByStatuses`, `GetExecutionMetrics` |
| `service_orders(customer_id)` | Histórico de OS por cliente, junção cliente ↔ OS na notificação | `OrderStatusNotifier`, relatórios |
| `service_orders(vehicle_id)` | OS por veículo, verificação de posse | `CreateServiceOrder` |
| `vehicles(customer_id)` | Veículos de um cliente, cascade de exclusão | `CreateVehicle`, `DeleteCustomer` |
| `service_order_items(service_order_id)` | Carregar itens ao detalhar uma OS | `FindByID` (preload) |
| `service_order_parts(service_order_id)` | Carregar peças ao detalhar/aprovar OS | `FindByID`, `ApproveBudget` |
| `customers(status)` | Filtrar clientes ativos; consulta da Lambda combina `document` (já único) e `status` | Lambda `auth-cpf`, listagens administrativas |

Chaves estrangeiras no PostgreSQL **não** criam índice automaticamente na coluna referenciadora; por isso os índices em `customer_id`, `vehicle_id` e `service_order_id` são explícitos. Todos são B-tree simples, de baixo custo de escrita para o volume esperado.

### 4.4 Parâmetros do RDS (repositório `oficina-infra-db`)

- `log_min_duration_statement = 500` — registra no CloudWatch qualquer consulta acima de 500 ms, insumo para detectar gargalos.
- Performance Insights habilitado — visão de espera por consulta sem instalar nada no banco.
- Backup automático de 1 dia e storage gp3 criptografado — suficiente para um ambiente que sobe e desce por sessão; em produção real o retention seria maior (ver RFC-002).

## 5. Consistência e desempenho

- **Transação na aprovação do orçamento.** `ApproveBudgetUseCase` muda o status, debita o estoque de cada peça e persiste a OS. O repositório executa a gravação da OS (cabeçalho + itens + peças) em uma única transação GORM; o débito de estoque é validado na entidade `Part` (`ErrPartInsufficientStock`) antes de qualquer `UPDATE`.
- **Sem soft delete.** Clientes, veículos, peças e serviços são removidos fisicamente; OS nunca são removidas por API (só saem da listagem ativa quando terminam). A coluna `deleted_at` prevista na convenção do projeto fica reservada para quando houver exigência de auditoria.
- **Timestamps.** Todas as tabelas de agregado têm `created_at` e `updated_at`; tabelas de item (`service_order_items`, `service_order_parts`) herdam o ciclo de vida da OS e por isso não os repetem.
- **Conexão.** A aplicação e a Lambda usam `sslmode=require` contra o RDS privado. O acesso é restrito por security group (nós do EKS e SG `db-client` da Lambda), sem IP público.
- **Unicidade.** `customers.document`, `vehicles.plate` e `users.email` são `UNIQUE`; a aplicação traduz a violação em `409 Conflict`.

## 6. Fase 4 — um banco por serviço

Cada microsserviço é dono dos seus dados e nenhum lê o banco do outro ([ADR-009](./adr/ADR-009-microsservicos-e-banco-por-servico.md)). Não existem chaves estrangeiras entre serviços: a ligação é o id da ordem de serviço, que viaja nas mensagens da saga.

| Serviço | Banco | Por quê |
|---|---|---|
| OS Service | PostgreSQL (RDS, `oficina-infra-db`) | Cadastros relacionais e a saga, que muda ordem, estado, histórico e outbox na mesma transação |
| Billing Service | PostgreSQL (RDS próprio) | Dinheiro: valores exatos (`DECIMAL`), unicidade de orçamento e de pagamento por ordem, transação com o outbox |
| Execution Service | DynamoDB | Ordem de trabalho lida e gravada inteira pela chave, sem junções; cobrança por requisição |

### 6.1 OS Service — migrations `000011` a `000015`

Nenhuma migration anterior foi alterada. A `000011` remove do OS o que passou a pertencer aos outros serviços (`parts`, `services`, `service_order_items`, `service_order_parts`) e acrescenta à ordem as referências ao orçamento e ao pagamento.

```mermaid
erDiagram
    customers ||--o{ vehicles : "possui"
    customers ||--o{ service_orders : "abre"
    vehicles ||--o{ service_orders : "e atendido em"
    service_orders ||--o{ service_order_status_history : "registra"
    service_orders ||--|| sagas : "e conduzida por"
    sagas ||--o{ saga_steps : "registra"

    service_orders {
        uuid id PK
        uuid customer_id FK
        uuid vehicle_id FK
        varchar status "received ... delivered ou cancelled"
        text notes
        decimal total_amount "vem do BudgetGenerated"
        uuid budget_id "id no Billing, sem FK"
        uuid payment_id "id no Billing, sem FK"
        text checkout_url
        text cancellation_reason
        timestamp started_at
        timestamp finished_at
    }

    service_order_status_history {
        uuid id PK
        uuid service_order_id FK
        varchar from_status
        varchar to_status
        text reason
        timestamptz occurred_at
    }

    sagas {
        uuid id PK
        uuid service_order_id FK "UNIQUE"
        varchar status "running, compensating, completed, compensated"
        varchar step "diagnosis, budget_approval, payment, execution"
        text pending_compensations
        text failure_reason
        timestamptz deadline
        int version "controle otimista"
        timestamptz finished_at
    }

    saga_steps {
        uuid id PK
        uuid saga_id FK
        varchar kind "command, reply, compensation, failure, outcome"
        varchar name
        text detail
        timestamptz occurred_at
    }

    outbox_messages {
        uuid id PK
        varchar destination
        varchar message_type
        uuid saga_id
        uuid order_id
        jsonb payload
        jsonb trace_context
        timestamptz occurred_at
        timestamptz published_at "nulo enquanto pendente"
    }

    processed_messages {
        uuid id PK "message_id recebido"
    }
```

| Tabela | Migration | Uso |
|---|---|---|
| `service_orders` (alterada) | `000011` | Perde os itens; ganha `budget_id`, `payment_id`, `checkout_url` e `cancellation_reason` |
| `service_order_status_history` | `000012` | Uma linha por mudança de status, com o motivo do cancelamento; atende `GET /service-orders/{id}/history` |
| `sagas` e `saga_steps` | `000013` | Estado da saga e registro de comandos, respostas, falhas e compensações; atende `GET /service-orders/{id}/saga` |
| `outbox_messages` | `000014` | Mensagens a publicar, gravadas na transação da mudança de estado |
| `processed_messages` | `000015` | Ids das mensagens já consumidas |

Índices: `sagas(deadline)` parcial para sagas `running` com prazo (busca das vencidas), `outbox_messages(created_at)` parcial para `published_at IS NULL` (fila do relay), `saga_steps(saga_id, occurred_at)` e `service_order_status_history(service_order_id, occurred_at)`.

### 6.2 Billing Service

```mermaid
erDiagram
    budgets ||--|{ budget_items : "contem"
    budgets ||--o| payments : "e cobrado por"

    budgets {
        uuid id PK
        uuid order_id "UNIQUE - ordem de servico no OS"
        uuid saga_id
        uuid customer_id
        varchar customer_name "copia recebida na mensagem"
        varchar customer_email "copia recebida na mensagem"
        varchar status "pending_approval, approved, rejected, cancelled"
        decimal total_amount
        text rejection_reason
        timestamptz decided_at
    }

    budget_items {
        uuid id PK
        uuid budget_id FK
        varchar kind "service ou part"
        varchar reference_id "id no catalogo do Execution"
        varchar description
        int quantity
        decimal unit_price "congelado no diagnostico"
    }

    payments {
        uuid id PK
        uuid budget_id FK
        uuid order_id "UNIQUE"
        uuid saga_id
        decimal amount
        varchar status "pending, confirmed, failed, refunded"
        varchar provider "mercadopago ou fake"
        varchar checkout_reference "id da preferencia"
        text checkout_url
        varchar provider_payment_id
        text failure_reason
    }
```

O Billing tem também as suas próprias `outbox_messages` e `processed_messages`, com a mesma estrutura das do OS. `order_id` é único em `budgets` e em `payments`: repetir `GenerateBudget` ou `RequestPayment` devolve o registro existente em vez de criar outro. O nome e o e-mail do cliente e os itens chegam copiados no comando `GenerateBudget`; o Billing não consulta o OS nem o Execution.

### 6.3 Execution Service — DynamoDB

Quatro tabelas com chave de partição `id` (string), sem chave de ordenação, cobrança por requisição ([RFC-007](./rfc/RFC-007-banco-nosql-do-execution.md)).

| Tabela | Item | Observação |
|---|---|---|
| `oficina-execution-work-orders` | Ordem de trabalho; `id` é o id da ordem de serviço | Serviços e peças do diagnóstico ficam **embutidos** no item, com nome e preço congelados |
| `oficina-execution-catalog` | Peça ou serviço do catálogo | A peça guarda o saldo de estoque |
| `oficina-execution-outbox` | Resposta da saga a publicar | Lida por varredura e apagada depois de publicada |
| `oficina-execution-processed-messages` | Id de mensagem já consumida | Consultada antes de processar cada mensagem |

As mudanças que precisam ser atômicas usam `TransactWriteItems`:

- **Reserva de estoque** (`StartExecution`): grava a ordem de trabalho em execução, debita cada peça com a condição de saldo suficiente e inclui `ExecutionStarted` no outbox. Se uma peça não tem saldo, nada é gravado e o serviço responde `ExecutionRejected`.
- **Devolução** (`CancelWorkOrder` com estoque reservado): cancela a ordem, devolve o saldo de cada peça e inclui `WorkOrderCancelled` no outbox.
- **Demais transições**: ordem de trabalho e mensagem de saída na mesma transação.

### 6.4 SQL x NoSQL

| Critério | PostgreSQL (OS e Billing) | DynamoDB (Execution) |
|---|---|---|
| Forma do dado | Vários agregados relacionados (cliente → veículo → ordem → saga) | Um agregado autocontido por chave |
| Consultas | Filtros e ordenações variados, histórico, junções | Leitura por id; listagem pequena por status |
| Consistência | Transação multi-tabela e chaves estrangeiras dentro do serviço | Transação de até 100 itens, suficiente para uma ordem e suas peças |
| Custo parado | Instância ligada por hora | Zero sem requisições |
| Esquema | Migrations versionadas | Sem esquema; tabelas criadas pelo Terraform |
