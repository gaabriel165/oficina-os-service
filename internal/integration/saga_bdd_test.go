package integration_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase/serviceorder"
	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
)

const (
	amountTolerance   = 0.001
	scenarioBudgetID  = "7d0a3c1b-2b1f-4f53-8a37-d1e6a7c49e55"
	scenarioPaymentID = "0f6f5c5e-8a53-4b0c-b3a2-6c1f6f7d9e21"
)

var destinationsByService = map[string]messaging.Destination{
	"execução":    messaging.DestinationExecutionCommands,
	"faturamento": messaging.DestinationBillingCommands,
}

type sagaScenario struct {
	t       *testing.T
	fixture *sagaFixture
}

func TestSagaFeatures(t *testing.T) {
	suite := godog.TestSuite{
		Name: "saga",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			registerSagaSteps(sc, &sagaScenario{t: t})
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
			Strict:   true,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("saga feature scenarios failed")
	}
}

func registerSagaSteps(sc *godog.ScenarioContext, s *sagaScenario) {
	sc.Step(`^um cliente com um veículo cadastrado$`, s.customerWithVehicle)
	sc.Step(`^o operador abre uma ordem de serviço para o veículo$`, s.orderIsOpened)
	sc.Step(`^que a ordem de serviço já tem um orçamento aguardando aprovação$`, s.orderIsWaitingApproval)
	sc.Step(`^que a ordem de serviço já tem uma cobrança aguardando pagamento$`, s.orderIsWaitingPayment)
	sc.Step(`^que a ordem de serviço já teve o pagamento confirmado$`, s.orderHasConfirmedPayment)

	sc.Step(`^o serviço de execução informa que o diagnóstico foi iniciado$`, s.diagnosisStarted)
	sc.Step(`^o serviço de execução conclui o diagnóstico com serviços e peças$`, s.diagnosisCompleted)
	sc.Step(`^o serviço de faturamento informa que o orçamento de (\d+\.\d+) foi gerado$`, s.budgetGenerated)
	sc.Step(`^o cliente aprova o orçamento$`, s.budgetApproved)
	sc.Step(`^o cliente recusa o orçamento por "([^"]*)"$`, s.budgetRejected)
	sc.Step(`^o serviço de faturamento informa que a cobrança foi criada$`, s.paymentRequested)
	sc.Step(`^o serviço de faturamento confirma o pagamento$`, s.paymentConfirmed)
	sc.Step(`^o serviço de faturamento informa que o pagamento foi recusado por "([^"]*)"$`, s.paymentFailed)
	sc.Step(`^o serviço de execução reserva o estoque e inicia o reparo$`, s.executionStarted)
	sc.Step(`^o serviço de execução rejeita o reparo por "([^"]*)"$`, s.executionRejected)
	sc.Step(`^o serviço de execução finaliza o reparo$`, s.executionFinished)
	sc.Step(`^o serviço de faturamento confirma o estorno do pagamento$`, s.paymentRefunded)
	sc.Step(`^o serviço de faturamento confirma o cancelamento do orçamento$`, s.budgetCancelled)
	sc.Step(`^o serviço de execução confirma o cancelamento da ordem de trabalho$`, s.workOrderCancelled)
	sc.Step(`^o operador entrega o veículo ao cliente$`, s.vehicleDelivered)

	sc.Step(`^a ordem de serviço fica com o status "([^"]*)"$`, s.orderHasStatus)
	sc.Step(`^a saga fica com o status "([^"]*)"$`, s.sagaHasStatus)
	sc.Step(`^o comando "([^"]*)" é enviado ao serviço de (execução|faturamento)$`, s.commandIsSent)
	sc.Step(`^o comando leva os dados do cliente e os itens do diagnóstico$`, s.commandCarriesCustomerAndDiagnosis)
	sc.Step(`^o valor total da ordem de serviço é (\d+\.\d+)$`, s.orderHasTotal)
	sc.Step(`^a ordem de serviço tem um link de pagamento$`, s.orderHasCheckoutLink)
	sc.Step(`^o motivo do cancelamento da ordem de serviço é "([^"]*)"$`, s.orderHasCancellationReason)
	sc.Step(`^o cliente foi notificado dos status "([^"]*)"$`, s.customerWasNotified)
	sc.Step(`^o histórico da ordem de serviço tem (\d+) mudanças de status$`, s.historyHasChanges)
	sc.Step(`^o registro da saga lista as compensações "([^"]*)"$`, s.sagaLogListsCompensations)
}

func (s *sagaScenario) customerWithVehicle() error {
	return nil
}

func (s *sagaScenario) orderIsOpened() error {
	s.fixture = newSagaFixture(s.t, time.Hour)
	return nil
}

func (s *sagaScenario) orderIsWaitingApproval() error {
	s.fixture.advanceToWaitingApproval(s.t)
	return nil
}

func (s *sagaScenario) orderIsWaitingPayment() error {
	s.fixture.advanceToWaitingPayment(s.t)
	return nil
}

func (s *sagaScenario) orderHasConfirmedPayment() error {
	s.fixture.advanceToWaitingPayment(s.t)
	return s.paymentConfirmed()
}

func (s *sagaScenario) reply(messageType string, payload any) error {
	message, err := messaging.NewMessage(messageType, s.fixture.saga.ID(), s.fixture.order.ID(), payload)
	if err != nil {
		return err
	}
	return s.fixture.orchestrator.Handle(s.fixture.ctx, message)
}

func (s *sagaScenario) diagnosisStarted() error {
	return s.reply(messaging.TypeDiagnosisStarted, messaging.WorkOrderPayload{WorkOrderID: s.fixture.order.ID()})
}

func (s *sagaScenario) diagnosisCompleted() error {
	return s.reply(messaging.TypeDiagnosisCompleted, messaging.DiagnosisCompletedPayload{
		WorkOrderID: s.fixture.order.ID(),
		Services:    []messaging.ServiceLine{{ServiceID: "service-1", Name: "Troca de óleo", LaborPrice: 120}},
		Parts:       []messaging.PartLine{{PartID: "part-1", Name: "Filtro de óleo", Quantity: 2, UnitPrice: 29.90}},
	})
}

func (s *sagaScenario) budgetGenerated(total float64) error {
	return s.reply(messaging.TypeBudgetGenerated, messaging.BudgetGeneratedPayload{BudgetID: scenarioBudgetID, TotalAmount: total})
}

func (s *sagaScenario) budgetApproved() error {
	return s.reply(messaging.TypeBudgetApproved, messaging.BudgetDecisionPayload{BudgetID: scenarioBudgetID})
}

func (s *sagaScenario) budgetRejected(reason string) error {
	return s.reply(messaging.TypeBudgetRejected, messaging.BudgetDecisionPayload{BudgetID: scenarioBudgetID, Reason: reason})
}

func (s *sagaScenario) paymentRequested() error {
	return s.reply(messaging.TypePaymentRequested, messaging.PaymentRequestedPayload{
		PaymentID: scenarioPaymentID, CheckoutURL: "https://pay.example/checkout",
	})
}

func (s *sagaScenario) paymentConfirmed() error {
	return s.reply(messaging.TypePaymentConfirmed, messaging.PaymentOutcomePayload{PaymentID: scenarioPaymentID})
}

func (s *sagaScenario) paymentFailed(reason string) error {
	return s.reply(messaging.TypePaymentFailed, messaging.PaymentOutcomePayload{PaymentID: scenarioPaymentID, Reason: reason})
}

func (s *sagaScenario) executionStarted() error {
	return s.reply(messaging.TypeExecutionStarted, messaging.WorkOrderPayload{WorkOrderID: s.fixture.order.ID()})
}

func (s *sagaScenario) executionRejected(reason string) error {
	return s.reply(messaging.TypeExecutionRejected, messaging.ReasonPayload{Reason: reason})
}

func (s *sagaScenario) executionFinished() error {
	return s.reply(messaging.TypeExecutionFinished, messaging.WorkOrderPayload{WorkOrderID: s.fixture.order.ID()})
}

func (s *sagaScenario) paymentRefunded() error {
	return s.reply(messaging.TypePaymentRefunded, messaging.PaymentOutcomePayload{PaymentID: scenarioPaymentID})
}

func (s *sagaScenario) budgetCancelled() error {
	return s.reply(messaging.TypeBudgetCancelled, nil)
}

func (s *sagaScenario) workOrderCancelled() error {
	return s.reply(messaging.TypeWorkOrderCancelled, nil)
}

func (s *sagaScenario) vehicleDelivered() error {
	_, err := serviceorder.NewDeliverVehicleUseCase(s.fixture.orders, s.fixture.notifier).Execute(s.fixture.ctx, s.fixture.order.ID())
	return err
}

func (s *sagaScenario) orderHasStatus(expected string) error {
	order, err := s.fixture.orders.FindByID(s.fixture.ctx, s.fixture.order.ID())
	if err != nil {
		return err
	}
	if order.Status().String() != expected {
		return fmt.Errorf("expected service order status %q, got %q", expected, order.Status())
	}
	return nil
}

func (s *sagaScenario) sagaHasStatus(expected string) error {
	current, err := s.fixture.sagas.FindByOrderID(s.fixture.ctx, s.fixture.order.ID())
	if err != nil {
		return err
	}
	if current.Status().String() != expected {
		return fmt.Errorf("expected saga status %q, got %q", expected, current.Status())
	}
	return nil
}

func (s *sagaScenario) lastCommand() (messaging.OutgoingMessage, error) {
	if _, err := s.fixture.relay.RelayOnce(s.fixture.ctx); err != nil {
		return messaging.OutgoingMessage{}, err
	}
	published := s.fixture.publisher.published
	if len(published) == 0 {
		return messaging.OutgoingMessage{}, fmt.Errorf("no command was published")
	}
	return published[len(published)-1], nil
}

func (s *sagaScenario) commandIsSent(commandType, service string) error {
	command, err := s.lastCommand()
	if err != nil {
		return err
	}
	if command.Message.Type != commandType {
		return fmt.Errorf("expected last command %q, got %q", commandType, command.Message.Type)
	}
	if command.Destination != destinationsByService[service] {
		return fmt.Errorf("expected command destination %q, got %q", destinationsByService[service], command.Destination)
	}
	return nil
}

func (s *sagaScenario) commandCarriesCustomerAndDiagnosis() error {
	command, err := s.lastCommand()
	if err != nil {
		return err
	}
	var payload messaging.GenerateBudgetPayload
	if err := command.Message.DecodePayload(&payload); err != nil {
		return err
	}
	if payload.CustomerID != s.fixture.order.CustomerID() || payload.CustomerEmail != "maria@example.com" {
		return fmt.Errorf("unexpected customer in command: %+v", payload)
	}
	if len(payload.Services) != 1 || len(payload.Parts) != 1 || payload.Parts[0].Quantity != 2 {
		return fmt.Errorf("unexpected diagnosis items in command: %+v", payload)
	}
	return nil
}

func (s *sagaScenario) orderHasTotal(expected float64) error {
	order, err := s.fixture.orders.FindByID(s.fixture.ctx, s.fixture.order.ID())
	if err != nil {
		return err
	}
	if math.Abs(order.TotalAmount()-expected) > amountTolerance {
		return fmt.Errorf("expected total amount %.2f, got %.2f", expected, order.TotalAmount())
	}
	return nil
}

func (s *sagaScenario) orderHasCheckoutLink() error {
	order, err := s.fixture.orders.FindByID(s.fixture.ctx, s.fixture.order.ID())
	if err != nil {
		return err
	}
	if order.CheckoutURL() == "" {
		return fmt.Errorf("expected the service order to have a checkout link")
	}
	return nil
}

func (s *sagaScenario) orderHasCancellationReason(expected string) error {
	order, err := s.fixture.orders.FindByID(s.fixture.ctx, s.fixture.order.ID())
	if err != nil {
		return err
	}
	if order.CancellationReason() != expected {
		return fmt.Errorf("expected cancellation reason %q, got %q", expected, order.CancellationReason())
	}
	return nil
}

func (s *sagaScenario) customerWasNotified(expected string) error {
	statuses := make([]string, len(s.fixture.notifier.statuses))
	for index, status := range s.fixture.notifier.statuses {
		statuses[index] = status.String()
	}
	if notified := strings.Join(statuses, ", "); notified != expected {
		return fmt.Errorf("expected notifications %q, got %q", expected, notified)
	}
	return nil
}

func (s *sagaScenario) historyHasChanges(expected int) error {
	history, err := s.fixture.orders.FindHistory(s.fixture.ctx, s.fixture.order.ID())
	if err != nil {
		return err
	}
	if len(history) != expected {
		return fmt.Errorf("expected %d status changes, got %d", expected, len(history))
	}
	return nil
}

func (s *sagaScenario) sagaLogListsCompensations(expected string) error {
	current, err := s.fixture.sagas.FindByOrderID(s.fixture.ctx, s.fixture.order.ID())
	if err != nil {
		return err
	}
	compensations := []string{}
	for _, entry := range current.Log() {
		if entry.Kind == valueobject.SagaLogKindCompensation {
			compensations = append(compensations, entry.Name)
		}
	}
	if listed := strings.Join(compensations, ", "); listed != expected {
		return fmt.Errorf("expected compensations %q, got %q", expected, listed)
	}
	return nil
}
