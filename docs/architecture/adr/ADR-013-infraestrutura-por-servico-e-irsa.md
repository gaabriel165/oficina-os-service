# ADR-013 — Infraestrutura por serviço, IRSA e ambiente único

| | |
|---|---|
| **Status** | Aceita |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |
| **Substitui** | [ADR-004](./ADR-004-quatro-repositorios-e-remote-state.md), [ADR-005](./ADR-005-ambientes-por-namespace.md) |
| **Relacionado** | [components.md](../components.md), [ADR-009](./ADR-009-microsservicos-e-banco-por-servico.md) |

## Contexto

A Fase 4 pede que cada microsserviço tenha repositório, infraestrutura e pipeline próprios, com deploy independente no Kubernetes. Até a Fase 3 havia quatro repositórios: a aplicação, a infraestrutura do cluster, a do banco e a Lambda. Com três serviços, três bancos e três filas, era preciso decidir onde fica o Terraform de cada recurso, como os pods acessam SQS e DynamoDB e o que fazer com o ambiente de homologação.

## Decisão

**1. Cada serviço provisiona o que é só dele**

Os três repositórios de serviço ganham uma pasta `terraform/`, com estado próprio no bucket S3 compartilhado:

| Repositório | Estado | Recursos |
|---|---|---|
| `oficina-os-service` | `os/` | Fila `oficina-saga-replies` + DLQ, role do pod |
| `oficina-billing-service` | `billing/` | RDS PostgreSQL próprio, fila `oficina-billing-commands` + DLQ, role do pod, `database_url` no SSM |
| `oficina-execution-service` | `execution/` | Quatro tabelas DynamoDB, fila `oficina-execution-commands` + DLQ, role do pod |

O que é compartilhado continua nos repositórios de plataforma: VPC, cluster, ECR, roles do GitHub Actions e segredos comuns em `oficina-infra-k8s` (`k8s/`); o RDS do OS em `oficina-infra-db` (`db/`); gateway e Lambdas em `oficina-lambda-auth` (`lambda/`). Cada serviço lê do estado `k8s/` a VPC, o security group dos nós e o provedor OIDC do cluster.

A fila pertence a **quem a consome**: o Billing é dono da fila de comandos dele, o OS é dono da fila de respostas.

**2. O pipeline do serviço faz tudo**

O CD de cada serviço, a cada push em `main`: aplica o próprio Terraform, publica a imagem no ECR, monta o `Secret` e faz o rollout. A execução manual com `destroy` remove o namespace e destrói a infraestrutura do serviço. Nenhum pipeline dispara outro.

**3. Acesso dos pods por IRSA**

Cada serviço tem uma IAM role assumida pelo seu `ServiceAccount` por meio do provedor OIDC do cluster (IAM Roles for Service Accounts). A política dá acesso apenas ao que o serviço usa: consumir a própria fila, publicar nas filas de destino e, no Execution, ler e gravar nas próprias tabelas. Não há chave de acesso em `Secret` nem permissão na role dos nós.

**4. Um ambiente só**

O namespace de homologação foi removido. Os serviços se comunicam por filas com nome fixo; um segundo ambiente no mesmo cluster consumiria as mesmas filas com outro banco e corromperia as sagas. Isolar de verdade exigiria duplicar filas, bancos e os três serviços. A validação antes de produção passou para o CI (testes de integração com banco e fila reais, BDD, gate de cobertura e quality gate) e para a execução local dos três serviços com `make system/up`.

**5. Rede e nós**

Um NLB por serviço, descoberto pelo Terraform do gateway pela tag `kubernetes.io/service-name`. Os nós passaram de `t3.small` para `t3.medium`: três workloads mais os agentes do New Relic não cabem no limite de 11 pods por nó do `t3.small`.

## Consequências

**Positivas**
- Quem altera um serviço altera, no mesmo pull request, o código, os manifests e a infraestrutura dele; criar ou destruir um serviço não toca nos outros.
- Privilégio mínimo por pod; credenciais temporárias e sem rotação manual.
- Sem ambiente duplicado, o custo por hora e o tempo de provisionamento não dobram.

**Negativas**
- As roles de deploy dos três serviços precisam de permissão para criar IAM, RDS, SQS e DynamoDB; hoje têm acesso de administrador, como as dos repositórios de infraestrutura. Em um ambiente permanente seriam restritas por prefixo de recurso.
- Há uma ordem de criação a respeitar (plataforma → banco do OS → serviços → gateway) e uma dependência circular pequena: o Billing precisa da URL do gateway, que só existe depois dos serviços; o deploy do Billing é executado de novo após o gateway.
- Três NLBs em vez de um. Um Ingress único reduziria o custo, mas exigiria um controlador a mais no cluster.
- Sem homologação, uma mudança aprovada no CI vai direto para produção; a mitigação é o conjunto de testes e o rollout com probes.
- O NLB de cada serviço é público; a rota `/internal` do OS fica acessível por ele e é protegida apenas pelo segredo compartilhado. Em produção real os NLBs seriam internos, com VPC Link no gateway.

## Alternativas rejeitadas

- **Todo o Terraform dos serviços em `oficina-infra-k8s`** — mais simples de aplicar, mas contraria a exigência de infraestrutura por serviço e acopla o deploy de um serviço ao pipeline da plataforma.
- **Um repositório de infraestrutura por serviço** — nove repositórios para manter sem ganho sobre a pasta `terraform/`.
- **Permissões na role dos nós** — qualquer pod do cluster herdaria acesso a todas as filas e tabelas.
- **EKS Pod Identity** — equivalente ao IRSA, mas exige um add-on a mais no cluster; o provedor OIDC já existia.
- **Homologação com prefixo nas filas e bancos próprios** — viável, mas dobraria os recursos e os pipelines para um ambiente que só existe durante a demonstração.
