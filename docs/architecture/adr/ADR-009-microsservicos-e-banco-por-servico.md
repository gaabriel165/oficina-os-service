# ADR-009 — Três microsserviços e um banco por serviço

| | |
|---|---|
| **Status** | Aceita |
| **Data** | 2026-11-16 |
| **Autor** | Gabriel Camargo |
| **Substitui** | [ADR-001](./ADR-001-comunicacao-sincrona-rest.md) (em parte) |
| **Relacionado** | [RFC-006](../rfc/RFC-006-mensageria.md), [RFC-007](../rfc/RFC-007-banco-nosql-do-execution.md), [ADR-010](./ADR-010-saga-orquestrada.md) |

## Contexto

Até a Fase 3 o sistema era um monólito modular com um único PostgreSQL. A Fase 4 exige ao menos três microsserviços, cada um com repositório, infraestrutura e banco próprios, com pelo menos um banco relacional e um NoSQL, comunicando-se por REST e por mensageria.

## Decisão

Dividir o monólito pelos três momentos do atendimento, que já tinham regras, atores e ritmos diferentes:

| Serviço | Responsabilidade | Atores | Banco |
|---|---|---|---|
| **OS Service** (`oficina-os-service`, o antigo monólito) | Clientes, veículos, ordem de serviço, status visível ao cliente e orquestração da saga | Atendente, cliente | PostgreSQL (RDS) |
| **Billing Service** (`oficina-billing-service`) | Orçamento, aprovação ou recusa, cobrança e estorno | Cliente, Mercado Pago | PostgreSQL (RDS próprio) |
| **Execution Service** (`oficina-execution-service`) | Diagnóstico, fila de execução, catálogo de serviços e peças, estoque | Mecânico | DynamoDB |

Regras que valem para os três:

- **Nenhum serviço lê o banco de outro.** Não há chave estrangeira entre serviços; a ligação é o id da ordem de serviço.
- **Dados de outro serviço chegam copiados na mensagem.** O diagnóstico vai para o Billing com nome e preço de cada item; o cliente vai com nome e e-mail. O Billing não consulta o OS nem o Execution.
- **Comunicação entre serviços por mensageria** (comandos e respostas da saga em SQS). **REST** fica para pessoas, para o webhook do Mercado Pago e para uma única chamada síncrona entre componentes: a Lambda `auth-cpf` consultando o cliente no OS Service.
- **Mesma arquitetura interna** (Clean Architecture) e os mesmos pacotes de mensageria, copiados em cada repositório em vez de compartilhados por biblioteca.
- **Mesmo JWT HS256** aceito pelos três, validado no gateway e em cada serviço.

## Consequências

**Positivas**
- Cada serviço é implantado, escalado e testado sozinho; uma falha no Billing não impede abrir ordens nem fazer diagnósticos.
- O banco de cada serviço foi escolhido pelo formato do dado ([database.md](../database.md), seção 6).
- A fronteira de cada serviço coincide com um ator e com um passo da saga, o que mantém os contratos pequenos.

**Negativas**
- Não há mais transação única: a consistência entre serviços é eventual e depende da saga e das compensações ([ADR-010](./ADR-010-saga-orquestrada.md)).
- Dados copiados podem ficar desatualizados (um cliente que troca de e-mail depois do orçamento gerado); aceito, porque orçamento é um retrato do momento.
- Código de mensageria duplicado em três repositórios; uma mudança de contrato exige alterar os três, começando por [`messaging-contracts.md`](../messaging-contracts.md).
- Mais infraestrutura e mais pipelines para manter ([ADR-013](./ADR-013-infraestrutura-por-servico-e-irsa.md)).

## Alternativas rejeitadas

- **Dois serviços (OS e Execution), com orçamento no OS** — não atende o mínimo de três e deixaria o pagamento, que tem integração externa e ritmo próprio, misturado ao cadastro.
- **Separar também clientes e veículos em um quarto serviço** — o cadastro só existe para abrir ordens; separá-lo criaria uma chamada síncrona a mais em toda abertura sem ganho de autonomia.
- **Banco compartilhado com um schema por serviço** — mais barato, mas mantém o acoplamento por dados que a fase pede para eliminar e impede escolher o tipo de banco por serviço.
- **Biblioteca compartilhada para a mensageria** — exigiria um sétimo repositório e versionamento coordenado; a cópia é pequena e estável.
