# language: pt
Funcionalidade: Saga da ordem de serviço
  Como oficina mecânica
  Quero que a ordem de serviço avance pelos serviços de execução e de faturamento
  Para que o cliente acompanhe o reparo da abertura à entrega
  E para que nada fique inconsistente quando uma etapa falha

  Contexto:
    Dado um cliente com um veículo cadastrado
    Quando o operador abre uma ordem de serviço para o veículo

  Cenário: Ordem de serviço concluída e entregue ao cliente
    Então a ordem de serviço fica com o status "received"
    E o comando "RequestDiagnosis" é enviado ao serviço de execução

    Quando o serviço de execução informa que o diagnóstico foi iniciado
    Então a ordem de serviço fica com o status "in_diagnosis"

    Quando o serviço de execução conclui o diagnóstico com serviços e peças
    Então o comando "GenerateBudget" é enviado ao serviço de faturamento
    E o comando leva os dados do cliente e os itens do diagnóstico

    Quando o serviço de faturamento informa que o orçamento de 179.80 foi gerado
    Então a ordem de serviço fica com o status "waiting_approval"
    E o valor total da ordem de serviço é 179.80

    Quando o cliente aprova o orçamento
    Então o comando "RequestPayment" é enviado ao serviço de faturamento

    Quando o serviço de faturamento informa que a cobrança foi criada
    Então a ordem de serviço fica com o status "waiting_payment"
    E a ordem de serviço tem um link de pagamento

    Quando o serviço de faturamento confirma o pagamento
    Então o comando "StartExecution" é enviado ao serviço de execução

    Quando o serviço de execução reserva o estoque e inicia o reparo
    Então a ordem de serviço fica com o status "in_execution"

    Quando o serviço de execução finaliza o reparo
    Então a ordem de serviço fica com o status "finished"
    E a saga fica com o status "completed"

    Quando o operador entrega o veículo ao cliente
    Então a ordem de serviço fica com o status "delivered"
    E o cliente foi notificado dos status "in_diagnosis, waiting_approval, waiting_payment, in_execution, finished, delivered"
    E o histórico da ordem de serviço tem 7 mudanças de status

  Cenário: Estoque insuficiente depois do pagamento desfaz tudo o que já foi feito
    Dado que a ordem de serviço já teve o pagamento confirmado
    Quando o serviço de execução rejeita o reparo por "insufficient stock quantity"
    Então a ordem de serviço fica com o status "cancelled"
    E a saga fica com o status "compensating"
    E o comando "RefundPayment" é enviado ao serviço de faturamento

    Quando o serviço de faturamento confirma o estorno do pagamento
    Então o comando "CancelBudget" é enviado ao serviço de faturamento

    Quando o serviço de faturamento confirma o cancelamento do orçamento
    Então o comando "CancelWorkOrder" é enviado ao serviço de execução

    Quando o serviço de execução confirma o cancelamento da ordem de trabalho
    Então a saga fica com o status "compensated"
    E o motivo do cancelamento da ordem de serviço é "insufficient stock quantity"
    E o registro da saga lista as compensações "RefundPayment, CancelBudget, CancelWorkOrder"

  Cenário: Orçamento recusado pelo cliente cancela a ordem de trabalho
    Dado que a ordem de serviço já tem um orçamento aguardando aprovação
    Quando o cliente recusa o orçamento por "valor acima do esperado"
    Então a ordem de serviço fica com o status "cancelled"
    E o comando "CancelWorkOrder" é enviado ao serviço de execução

    Quando o serviço de execução confirma o cancelamento da ordem de trabalho
    Então a saga fica com o status "compensated"
    E o motivo do cancelamento da ordem de serviço é "valor acima do esperado"

  Cenário: Pagamento recusado cancela o orçamento e a ordem de trabalho
    Dado que a ordem de serviço já tem uma cobrança aguardando pagamento
    Quando o serviço de faturamento informa que o pagamento foi recusado por "cartão recusado"
    Então a ordem de serviço fica com o status "cancelled"
    E o comando "CancelBudget" é enviado ao serviço de faturamento

    Quando o serviço de faturamento confirma o cancelamento do orçamento
    Então o comando "CancelWorkOrder" é enviado ao serviço de execução

    Quando o serviço de execução confirma o cancelamento da ordem de trabalho
    Então a saga fica com o status "compensated"
