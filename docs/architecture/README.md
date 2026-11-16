# Documentação de Arquitetura — Tech Challenge Fases 3 e 4

| | |
|---|---|
| **Projeto** | Oficina Mecânica — Sistema Integrado de Atendimento e Execução de Serviços |
| **Fase** | 3 — Segurança, serverless, CI/CD segregado e observabilidade · 4 — Microsserviços, mensageria e saga |
| **Status** | Vigente |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |

Esta pasta reúne a documentação arquitetural do projeto. Ela nasceu na Fase 3 e foi atualizada na Fase 4, quando o monólito foi dividido em três microsserviços (OS, Billing e Execution) coordenados por uma saga orquestrada. Ela foi organizada em níveis de zoom, do mais amplo ao mais específico, inspirada no modelo C4: primeiro **onde** o sistema vive e com quem conversa, depois **como** as peças se comunicam, em seguida **quais dados** persistimos e, por fim, **por que** cada decisão foi tomada.

## Como ler

1. **Contexto e componentes** → [`components.md`](./components.md)
   Visão de nuvem com API Gateway, Lambdas, os três microsserviços no EKS, filas SQS, bancos, Mercado Pago, New Relic e os seis repositórios. Inclui as camadas internas do OS Service (Clean Architecture) e a ordem de deploy.
2. **Contratos de mensagens** → [`messaging-contracts.md`](./messaging-contracts.md)
   Fonte única das filas, do envelope e de cada comando e resposta trocados entre os serviços.
3. **Fluxos (diagramas de sequência)**
   - [`sequence-service-order.md`](./sequence-service-order.md) — saga da ordem de serviço: caminho feliz, compensações, estados e garantias (outbox, idempotência, prazo).
   - [`sequence-authentication.md`](./sequence-authentication.md) — autenticação de cliente por CPF (Lambda consultando o OS Service) e de operador, validação do token no gateway e nos serviços.
4. **Dados** → [`database.md`](./database.md)
   Justificativa do PostgreSQL, diagrama ER do monólito (histórico) e, na seção 6, o modelo vigente com um banco por serviço: PostgreSQL no OS e no Billing, DynamoDB no Execution.
5. **Decisões**
   - **RFCs** (decisões técnicas discutíveis, com opções comparadas) em [`rfc/`](./rfc/).
   - **ADRs** (decisões arquiteturais permanentes) em [`adr/`](./adr/).
6. **Diagramas por repositório** → [`repo-diagrams.md`](./repo-diagrams.md)
   Um diagrama pequeno para o README de cada repositório.

## RFCs — Request for Comments

| # | Título | Status |
|---|---|---|
| [RFC-001](./rfc/RFC-001-escolha-da-nuvem.md) | Escolha da nuvem | Aceita |
| [RFC-002](./rfc/RFC-002-escolha-do-banco-de-dados.md) | Escolha do banco de dados | Aceita |
| [RFC-003](./rfc/RFC-003-estrategia-de-autenticacao.md) | Estratégia de autenticação | Aceita |
| [RFC-004](./rfc/RFC-004-ferramenta-de-observabilidade.md) | Ferramenta de observabilidade | Aceita |
| [RFC-005](./rfc/RFC-005-api-gateway.md) | API Gateway | Aceita |
| [RFC-006](./rfc/RFC-006-mensageria.md) | Mensageria entre os microsserviços | Aceita |
| [RFC-007](./rfc/RFC-007-banco-nosql-do-execution.md) | Banco NoSQL do Execution Service | Aceita |

## ADRs — Architecture Decision Records

| # | Título | Status |
|---|---|---|
| [ADR-001](./adr/ADR-001-comunicacao-sincrona-rest.md) | Comunicação síncrona via REST | Substituída por ADR-009 e ADR-010 |
| [ADR-002](./adr/ADR-002-hpa-por-cpu.md) | Autoescala horizontal por CPU (HPA) | Aceita |
| [ADR-003](./adr/ADR-003-jwt-hs256-segredo-compartilhado.md) | JWT HS256 com segredo compartilhado via SSM | Aceita |
| [ADR-004](./adr/ADR-004-quatro-repositorios-e-remote-state.md) | Quatro repositórios e remote state no S3 | Substituída por ADR-013 |
| [ADR-005](./adr/ADR-005-ambientes-por-namespace.md) | Homologação e produção como namespaces | Substituída por ADR-013 |
| [ADR-006](./adr/ADR-006-logs-estruturados-com-correlacao.md) | Logs estruturados em JSON com correlação | Aceita |
| [ADR-007](./adr/ADR-007-lambda-authorizer-na-borda.md) | Lambda authorizer na borda | Aceita |
| [ADR-008](./adr/ADR-008-migrations-no-startup-da-aplicacao.md) | Migrations executadas no startup da aplicação | Aceita |
| [ADR-009](./adr/ADR-009-microsservicos-e-banco-por-servico.md) | Três microsserviços e um banco por serviço | Aceita |
| [ADR-010](./adr/ADR-010-saga-orquestrada.md) | Saga orquestrada no OS Service | Aceita |
| [ADR-011](./adr/ADR-011-outbox-transacional-e-consumo-idempotente.md) | Outbox transacional e consumo idempotente | Aceita |
| [ADR-012](./adr/ADR-012-pagamento-com-mercado-pago-checkout-pro.md) | Pagamento com Mercado Pago (Checkout Pro) | Aceita |
| [ADR-013](./adr/ADR-013-infraestrutura-por-servico-e-irsa.md) | Infraestrutura por serviço, IRSA e ambiente único | Aceita |

## Os seis repositórios

| Repositório | Responsabilidade | Provisiona / entrega |
|---|---|---|
| [`oficina-os-service`](https://github.com/gaabriel165/oficina-os-service) | Ordens de serviço, clientes, veículos e orquestração da saga | Imagem no ECR, deploy no namespace `oficina-api`, fila `oficina-saga-replies` e role do pod |
| [`oficina-billing-service`](https://github.com/gaabriel165/oficina-billing-service) | Orçamento e pagamento (Mercado Pago) | Imagem, deploy no namespace `oficina-billing`, RDS próprio, fila `oficina-billing-commands` e role do pod |
| [`oficina-execution-service`](https://github.com/gaabriel165/oficina-execution-service) | Diagnóstico, execução, catálogo e estoque | Imagem, deploy no namespace `oficina-execution`, tabelas DynamoDB, fila `oficina-execution-commands` e role do pod |
| [`oficina-infra-k8s`](https://github.com/gaabriel165/oficina-infra-k8s) | Plataforma compartilhada (Terraform) | VPC, EKS 1.34, ECR, OIDC + roles do GitHub Actions, `jwt_secret` e `internal_api_secret` no SSM, metrics-server e New Relic via Helm |
| [`oficina-infra-db`](https://github.com/gaabriel165/oficina-infra-db) | Banco do OS Service (Terraform) | RDS PostgreSQL 16 privado, security group, `database_url` no SSM |
| [`oficina-lambda-auth`](https://github.com/gaabriel165/oficina-lambda-auth) | Functions serverless (Go) + API Gateway (Terraform) | Lambdas `auth-cpf` e `authorizer`, API Gateway roteando para os três serviços |

## Convenções

- Diagramas em **Mermaid**, renderizados nativamente pelo GitHub. Versões em PNG são geradas para o documento de entrega.
- Datas no formato ISO (`AAAA-MM-DD`).
- Recursos compartilhados e do OS Service prefixados por `oficina-api`; recursos do Billing e do Execution prefixados pelo nome do serviço. Parâmetros compartilhados no SSM sob `/oficina-api/`.
