# Teste de carga (k6)

Gera carga nas rotas protegidas para demonstrar o HPA escalando e alimentar os dashboards de latência.

```bash
# via API Gateway (token emitido pela Lambda auth-cpf)
k6 run -e BASE_URL=https://<api-id>.execute-api.us-east-1.amazonaws.com load/load-test.js

# acompanhar a autoescala
kubectl get hpa -n oficina-api -w
```

Rampa: 10 → 30 → 60 usuários virtuais em 4,5 minutos. Cada iteração lista OS como cliente (token da Lambda de CPF, só as próprias) e como operador, consulta as métricas de execução no Execution Service (roteadas pelo gateway) e faz um login (bcrypt consome CPU).
