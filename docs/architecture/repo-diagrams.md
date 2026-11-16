# Diagramas por repositório

| | |
|---|---|
| **Status** | Vigente |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |

Cada bloco abaixo é o diagrama de arquitetura específico de um repositório, pronto para ser colado na seção "Arquitetura" do respectivo `README.md`.

## 1. `oficina-os-service` — ordens de serviço e orquestrador da saga

```mermaid
flowchart LR
    subgraph GitHub["GitHub - oficina-os-service"]
        PR["Pull Request"] --> CI["CI: testes, cobertura >= 80%, SonarCloud, kustomize, terraform validate"]
        Main["branch main"] --> CD["CD: terraform apply, imagem, rollout"]
    end

    subgraph AWS["AWS"]
        ECR[("ECR")]
        SSM[("SSM: database_url, jwt_secret, internal_api_secret")]
        QR[["SQS saga-replies + dlq"]]
        QC[["SQS billing-commands e execution-commands"]]
        Role["IAM role do pod (IRSA)"]
        subgraph EKS["EKS - namespace oficina-api"]
            Dep["Deployment 2..6 réplicas + HPA"]
            Svc["Service LoadBalancer - NLB"]
        end
        RDS[("RDS PostgreSQL")]
    end

    NR["New Relic"]
    Resend["Resend"]
    BrasilAPI["BrasilAPI"]

    CD -->|"terraform apply"| QR
    CD -->|"terraform apply"| Role
    CD -->|"docker build e push"| ECR
    CD -.->|"lê segredos"| SSM
    CD -->|"kubectl apply -k"| Dep
    Svc --> Dep
    Dep --> RDS
    Dep -->|"comandos"| QC
    QR -->|"respostas"| Dep
    Dep -.->|"assume"| Role
    Dep --> Resend
    Dep --> BrasilAPI
    Dep -.-> NR
```

## 2. `oficina-infra-k8s` — infraestrutura Kubernetes

```mermaid
flowchart LR
    subgraph GitHub["GitHub - oficina-infra-k8s"]
        PR["Pull Request"] --> Plan["terraform fmt, validate, plan"]
        Main["branch main"] --> Apply["terraform apply + helm"]
    end

    S3[("S3 state - k8s/")]

    subgraph AWS["AWS us-east-1"]
        subgraph VPC["VPC 10.0.0.0/16"]
            Pub["Subnets públicas - NAT Gateway"]
            Priv["Subnets privadas"]
            subgraph EKS["EKS 1.34"]
                NG["Managed node group t3.medium 2..4 - AL2023"]
                MS["metrics-server"]
                NRB["New Relic nri-bundle"]
            end
        end
        ECR[("ECR: um repositório por microsserviço")]
        OIDC["OIDC provider GitHub"]
        Roles["6 roles IAM: os, billing, execution, k8s-infra, db-infra, lambda"]
        SSM[("SSM: jwt_secret, internal_api_secret")]
    end

    Apply -.-> S3
    Apply --> VPC
    Apply --> EKS
    Apply --> ECR
    Apply --> OIDC
    Apply --> Roles
    Apply --> SSM
    Apply -->|"helm upgrade --install"| MS
    Apply -->|"helm upgrade --install"| NRB
    OIDC --> Roles
    Roles -.->|"assumidas via OIDC"| GitHub
    S3 -.->|"remote state lido por"| Consumers["os três microsserviços, oficina-infra-db e oficina-lambda-auth"]
```

## 3. `oficina-infra-db` — banco de dados gerenciado

```mermaid
flowchart LR
    subgraph GitHub["GitHub - oficina-infra-db"]
        PR["Pull Request"] --> Plan["terraform fmt, validate, plan"]
        Main["branch main"] --> Apply["terraform apply"]
    end

    S3k[("S3 state - k8s/")]
    S3d[("S3 state - db/")]

    subgraph AWS["AWS us-east-1"]
        subgraph VPC["VPC - subnets privadas"]
            RDS[("RDS PostgreSQL 16.15 db.t3.micro<br/>gp3 20GB criptografado, backup 1 dia")]
            SGdb["SG db: 5432 apenas do SG dos nós EKS"]
            SGclient["SG db-client - sem uso desde a Fase 4"]
            PG["Parameter group: log_min_duration_statement 500"]
        end
        SSM[("SSM: /oficina-api/database_url")]
        CW["CloudWatch: logs postgresql, Performance Insights"]
    end

    Apply -.->|"lê vpc, subnets, SG dos nós"| S3k
    Apply -.-> S3d
    Apply --> RDS
    Apply --> SGdb
    Apply --> SGclient
    Apply --> PG
    Apply --> SSM
    RDS -.-> CW
    SSM -.->|"lido pelo CD do OS Service"| Consumers["oficina-os-service"]
```

## 4. `oficina-lambda-auth` — functions serverless e API Gateway

```mermaid
flowchart LR
    subgraph GitHub["GitHub - oficina-lambda-auth"]
        PR["Pull Request"] --> CI["go test, go build arm64, terraform plan"]
        Main["branch main"] --> CD["build zips + terraform apply"]
    end

    S3["S3 state - k8s/, lambda/"]

    subgraph AWS["AWS us-east-1"]
        APIGW["API Gateway HTTP API"]
        LA["Lambda auth-cpf<br/>provided.al2023 arm64, fora da VPC"]
        LZ["Lambda authorizer<br/>REQUEST, simple response, cache 300s"]
        SSM[("SSM: jwt_secret, internal_api_secret, api_gateway_url")]
        NLBOS["NLB do OS Service"]
        NLBB["NLB do Billing Service"]
        NLBE["NLB do Execution Service"]
        CW["CloudWatch Logs"]
    end

    Cliente["Cliente"] -->|"POST /auth/cpf"| APIGW
    Operador["Operador"] -->|"/api/v1/** Bearer JWT"| APIGW
    MP["Mercado Pago"] -->|"POST /api/v1/webhooks/mercadopago"| APIGW
    APIGW -->|"AWS_PROXY"| LA
    APIGW -.->|"rotas protegidas"| LZ
    APIGW -->|"demais rotas /api/v1"| NLBOS
    APIGW -->|"budgets, payments, webhook"| NLBB
    APIGW -->|"work-orders, parts, services"| NLBE
    LA -->|"GET /internal/customers/by-document"| NLBOS
    CD -.-> S3
    CD --> LA
    CD --> LZ
    CD --> APIGW
    CD -->|"publica a URL do gateway"| SSM
    LA -.->|"env resolvido pelo Terraform"| SSM
    LZ -.->|"env resolvido pelo Terraform"| SSM
    LA -.-> CW
    LZ -.-> CW
```

## 5. `oficina-billing-service` — orçamento e pagamento

```mermaid
flowchart LR
    subgraph GitHub["GitHub - oficina-billing-service"]
        PR["Pull Request"] --> CI["CI: testes, cobertura >= 80%, SonarCloud, kustomize, terraform validate"]
        Main["branch main"] --> CD["CD: terraform apply, imagem, rollout"]
    end

    subgraph AWS["AWS"]
        ECR[("ECR")]
        SSM[("SSM: database_url do billing, jwt_secret, api_gateway_url")]
        QB[["SQS billing-commands + dlq"]]
        QR[["SQS saga-replies"]]
        Role["IAM role do pod (IRSA)"]
        subgraph EKS["EKS - namespace oficina-billing"]
            Dep["Deployment 1..4 réplicas + HPA"]
            Svc["Service LoadBalancer - NLB"]
        end
        RDS[("RDS PostgreSQL próprio")]
    end

    MP["Mercado Pago"]
    Resend["Resend"]
    NR["New Relic"]

    CD -->|"terraform apply"| RDS
    CD -->|"terraform apply"| QB
    CD -->|"terraform apply"| Role
    CD -->|"docker build e push"| ECR
    CD -.->|"lê segredos"| SSM
    CD -->|"kubectl apply -k"| Dep
    Svc --> Dep
    QB -->|"comandos"| Dep
    Dep -->|"respostas"| QR
    Dep --> RDS
    Dep -->|"preferência, consulta, estorno"| MP
    MP -->|"webhook assinado"| Svc
    Dep --> Resend
    Dep -.->|"assume"| Role
    Dep -.-> NR
```

## 6. `oficina-execution-service` — diagnóstico, execução e estoque

```mermaid
flowchart LR
    subgraph GitHub["GitHub - oficina-execution-service"]
        PR["Pull Request"] --> CI["CI: testes, cobertura >= 80%, SonarCloud, kustomize, terraform validate"]
        Main["branch main"] --> CD["CD: terraform apply, imagem, rollout"]
    end

    subgraph AWS["AWS"]
        ECR[("ECR")]
        SSM[("SSM: jwt_secret")]
        QE[["SQS execution-commands + dlq"]]
        QR[["SQS saga-replies"]]
        Role["IAM role do pod (IRSA)"]
        DDB[("DynamoDB: work-orders, catalog, outbox, processed-messages")]
        subgraph EKS["EKS - namespace oficina-execution"]
            Dep["Deployment 1..4 réplicas + HPA"]
            Svc["Service LoadBalancer - NLB"]
        end
    end

    NR["New Relic"]

    CD -->|"terraform apply"| DDB
    CD -->|"terraform apply"| QE
    CD -->|"terraform apply"| Role
    CD -->|"docker build e push"| ECR
    CD -.->|"lê segredos"| SSM
    CD -->|"kubectl apply -k"| Dep
    Svc --> Dep
    QE -->|"comandos"| Dep
    Dep -->|"respostas"| QR
    Dep --> DDB
    Dep -.->|"assume"| Role
    Dep -.-> NR
```
