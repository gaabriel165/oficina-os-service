package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/application/outbox"
	"github.com/gabrielcamargo/oficina-api/internal/application/saga"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase/serviceorder"
	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/domain/repository"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
	infrarepo "github.com/gabrielcamargo/oficina-api/internal/infrastructure/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturingPublisher struct {
	published []messaging.OutgoingMessage
}

func (p *capturingPublisher) Publish(_ context.Context, destination messaging.Destination, message messaging.Message) error {
	p.published = append(p.published, messaging.OutgoingMessage{Destination: destination, Message: message})
	return nil
}

func (p *capturingPublisher) types() []string {
	types := make([]string, len(p.published))
	for index, outgoing := range p.published {
		types[index] = outgoing.Message.Type
	}
	return types
}

type countingNotifier struct {
	statuses []valueobject.OrderStatus
}

func (n *countingNotifier) Notify(order *entity.ServiceOrder) {
	n.statuses = append(n.statuses, order.Status())
}

type sagaFixture struct {
	ctx          context.Context
	orders       *infrarepo.GormServiceOrderRepository
	sagas        *infrarepo.GormSagaRepository
	orchestrator *saga.Orchestrator
	relay        *outbox.Relay
	publisher    *capturingPublisher
	notifier     *countingNotifier
	order        *entity.ServiceOrder
	saga         *entity.Saga
}

func newSagaFixture(t *testing.T, stepTimeout time.Duration) *sagaFixture {
	t.Helper()
	db := newTestDB(t)
	ctx := context.Background()
	customers := infrarepo.NewGormCustomerRepository(db)
	vehicles := infrarepo.NewGormVehicleRepository(db)

	customer, err := entity.NewCustomer("Maria Silva", "529.982.247-25", "11999999999", "maria@example.com")
	require.NoError(t, err)
	require.NoError(t, customers.Create(customer))
	vehicle, err := entity.NewVehicle(customer.ID(), "BRA2E19", "Honda", "Civic", 2022)
	require.NoError(t, err)
	require.NoError(t, vehicles.Create(vehicle))

	fixture := &sagaFixture{
		ctx:       ctx,
		orders:    infrarepo.NewGormServiceOrderRepository(db),
		sagas:     infrarepo.NewGormSagaRepository(db),
		publisher: &capturingPublisher{},
		notifier:  &countingNotifier{},
	}
	fixture.orchestrator = saga.NewOrchestrator(fixture.sagas, fixture.orders, customers, fixture.notifier, stepTimeout)
	fixture.relay = outbox.NewRelay(infrarepo.NewGormOutboxRepository(db), fixture.publisher)

	order, err := serviceorder.NewCreateServiceOrderUseCase(customers, vehicles, fixture.orchestrator).Execute(ctx, serviceorder.CreateServiceOrderInput{
		CustomerID: customer.ID(), VehicleID: vehicle.ID(), Notes: "engine noise",
	})
	require.NoError(t, err)
	fixture.order = order

	started, err := fixture.sagas.FindByOrderID(ctx, order.ID())
	require.NoError(t, err)
	fixture.saga = started
	return fixture
}

func (f *sagaFixture) reply(t *testing.T, messageType string, payload any) {
	t.Helper()
	message, err := messaging.NewMessage(messageType, f.saga.ID(), f.order.ID(), payload)
	require.NoError(t, err)
	require.NoError(t, f.orchestrator.Handle(f.ctx, message))
}

func (f *sagaFixture) relayAll(t *testing.T) {
	t.Helper()
	_, err := f.relay.RelayOnce(f.ctx)
	require.NoError(t, err)
}

func (f *sagaFixture) currentOrder(t *testing.T) *entity.ServiceOrder {
	t.Helper()
	order, err := f.orders.FindByID(f.ctx, f.order.ID())
	require.NoError(t, err)
	return order
}

func (f *sagaFixture) currentSaga(t *testing.T) *entity.Saga {
	t.Helper()
	current, err := f.sagas.FindByOrderID(f.ctx, f.order.ID())
	require.NoError(t, err)
	return current
}

func (f *sagaFixture) advanceToWaitingApproval(t *testing.T) {
	t.Helper()
	f.reply(t, messaging.TypeDiagnosisStarted, messaging.WorkOrderPayload{WorkOrderID: f.order.ID()})
	f.reply(t, messaging.TypeDiagnosisCompleted, messaging.DiagnosisCompletedPayload{
		WorkOrderID: f.order.ID(),
		Services:    []messaging.ServiceLine{{ServiceID: uuid.NewString(), Name: "Oil change", LaborPrice: 120}},
		Parts:       []messaging.PartLine{{PartID: uuid.NewString(), Name: "Oil filter", Quantity: 2, UnitPrice: 29.90}},
	})
	f.reply(t, messaging.TypeBudgetGenerated, messaging.BudgetGeneratedPayload{BudgetID: uuid.NewString(), TotalAmount: 179.80})
}

func (f *sagaFixture) advanceToWaitingPayment(t *testing.T) {
	t.Helper()
	f.advanceToWaitingApproval(t)
	f.reply(t, messaging.TypeBudgetApproved, messaging.BudgetDecisionPayload{})
	f.reply(t, messaging.TypePaymentRequested, messaging.PaymentRequestedPayload{PaymentID: uuid.NewString(), CheckoutURL: "https://pay.example/checkout"})
}

func TestSaga_ShouldRequestDiagnosisWhenOrderIsOpened(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	fixture.relayAll(t)

	assert.Equal(t, valueobject.OrderStatusReceived, fixture.currentOrder(t).Status())
	assert.Equal(t, valueobject.SagaStatusRunning, fixture.saga.Status())
	assert.Equal(t, valueobject.SagaStepDiagnosis, fixture.saga.Step())
	require.Len(t, fixture.publisher.published, 1)
	assert.Equal(t, messaging.TypeRequestDiagnosis, fixture.publisher.published[0].Message.Type)
	assert.Equal(t, messaging.DestinationExecutionCommands, fixture.publisher.published[0].Destination)
}

func TestSaga_ShouldCompleteTheHappyPath(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	fixture.advanceToWaitingPayment(t)
	fixture.reply(t, messaging.TypePaymentConfirmed, messaging.PaymentOutcomePayload{})
	fixture.reply(t, messaging.TypeExecutionStarted, messaging.WorkOrderPayload{})
	fixture.reply(t, messaging.TypeExecutionFinished, messaging.WorkOrderPayload{})
	fixture.relayAll(t)

	order := fixture.currentOrder(t)
	current := fixture.currentSaga(t)
	history, err := fixture.orders.FindHistory(fixture.ctx, order.ID())
	require.NoError(t, err)

	assert.Equal(t, valueobject.OrderStatusFinished, order.Status())
	assert.InDelta(t, 179.80, order.TotalAmount(), 0.001)
	assert.NotEmpty(t, order.BudgetID())
	assert.Equal(t, "https://pay.example/checkout", order.CheckoutURL())
	assert.NotNil(t, order.StartedAt())
	assert.NotNil(t, order.FinishedAt())
	assert.Equal(t, valueobject.SagaStatusCompleted, current.Status())
	assert.NotNil(t, current.FinishedAt())
	assert.Equal(t, []string{
		messaging.TypeRequestDiagnosis,
		messaging.TypeGenerateBudget,
		messaging.TypeRequestPayment,
		messaging.TypeStartExecution,
	}, fixture.publisher.types())
	assert.Len(t, history, 6)
	assert.Equal(t, []valueobject.OrderStatus{
		valueobject.OrderStatusInDiagnosis,
		valueobject.OrderStatusWaitingApproval,
		valueobject.OrderStatusWaitingPayment,
		valueobject.OrderStatusInExecution,
		valueobject.OrderStatusFinished,
	}, fixture.notifier.statuses)
}

func TestSaga_ShouldSendCustomerDataWithTheBudgetCommand(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	fixture.advanceToWaitingApproval(t)
	fixture.relayAll(t)

	var payload messaging.GenerateBudgetPayload
	for _, outgoing := range fixture.publisher.published {
		if outgoing.Message.Type == messaging.TypeGenerateBudget {
			require.NoError(t, outgoing.Message.DecodePayload(&payload))
			assert.Equal(t, messaging.DestinationBillingCommands, outgoing.Destination)
		}
	}

	assert.Equal(t, "Maria Silva", payload.CustomerName)
	assert.Equal(t, "maria@example.com", payload.CustomerEmail)
	assert.Len(t, payload.Services, 1)
	assert.Len(t, payload.Parts, 1)
}

func TestSaga_ShouldCancelWorkOrderWhenBudgetIsRejected(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	fixture.advanceToWaitingApproval(t)

	fixture.reply(t, messaging.TypeBudgetRejected, messaging.BudgetDecisionPayload{Reason: "too expensive"})
	compensating := fixture.currentSaga(t)
	fixture.reply(t, messaging.TypeWorkOrderCancelled, nil)
	fixture.relayAll(t)

	order := fixture.currentOrder(t)
	compensated := fixture.currentSaga(t)
	assert.Equal(t, valueobject.SagaStatusCompensating, compensating.Status())
	assert.Equal(t, []string{messaging.TypeCancelWorkOrder}, compensating.PendingCompensations())
	assert.Equal(t, valueobject.OrderStatusCancelled, order.Status())
	assert.Equal(t, "too expensive", order.CancellationReason())
	assert.Equal(t, valueobject.SagaStatusCompensated, compensated.Status())
	assert.Empty(t, compensated.PendingCompensations())
	assert.Contains(t, fixture.publisher.types(), messaging.TypeCancelWorkOrder)
	assert.NotContains(t, fixture.publisher.types(), messaging.TypeCancelBudget)
}

func TestSaga_ShouldCancelBudgetAndWorkOrderWhenPaymentFails(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	fixture.advanceToWaitingPayment(t)

	fixture.reply(t, messaging.TypePaymentFailed, messaging.PaymentOutcomePayload{Reason: "card declined"})
	fixture.reply(t, messaging.TypeBudgetCancelled, nil)
	fixture.reply(t, messaging.TypeWorkOrderCancelled, nil)
	fixture.relayAll(t)

	assert.Equal(t, valueobject.OrderStatusCancelled, fixture.currentOrder(t).Status())
	assert.Equal(t, valueobject.SagaStatusCompensated, fixture.currentSaga(t).Status())
	assert.Equal(t, []string{
		messaging.TypeRequestDiagnosis,
		messaging.TypeGenerateBudget,
		messaging.TypeRequestPayment,
		messaging.TypeCancelBudget,
		messaging.TypeCancelWorkOrder,
	}, fixture.publisher.types())
}

func TestSaga_ShouldRefundPaymentWhenExecutionIsRejected(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	fixture.advanceToWaitingPayment(t)
	fixture.reply(t, messaging.TypePaymentConfirmed, messaging.PaymentOutcomePayload{})

	fixture.reply(t, messaging.TypeExecutionRejected, messaging.ReasonPayload{Reason: "insufficient stock quantity"})
	afterFailure := fixture.currentSaga(t)
	fixture.reply(t, messaging.TypePaymentRefunded, messaging.PaymentOutcomePayload{})
	afterRefund := fixture.currentSaga(t)
	fixture.reply(t, messaging.TypeBudgetCancelled, nil)
	fixture.reply(t, messaging.TypeWorkOrderCancelled, nil)
	fixture.relayAll(t)

	order := fixture.currentOrder(t)
	assert.Equal(t, []string{messaging.TypeRefundPayment, messaging.TypeCancelBudget, messaging.TypeCancelWorkOrder}, afterFailure.PendingCompensations())
	assert.Equal(t, []string{messaging.TypeCancelBudget, messaging.TypeCancelWorkOrder}, afterRefund.PendingCompensations())
	assert.Equal(t, valueobject.OrderStatusCancelled, order.Status())
	assert.Equal(t, "insufficient stock quantity", order.CancellationReason())
	assert.Equal(t, valueobject.SagaStatusCompensated, fixture.currentSaga(t).Status())
	assert.Equal(t, []string{
		messaging.TypeRequestDiagnosis,
		messaging.TypeGenerateBudget,
		messaging.TypeRequestPayment,
		messaging.TypeStartExecution,
		messaging.TypeRefundPayment,
		messaging.TypeCancelBudget,
		messaging.TypeCancelWorkOrder,
	}, fixture.publisher.types())
}

func TestSaga_ShouldEndWithoutCompensationWhenDiagnosisFails(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)

	fixture.reply(t, messaging.TypeDiagnosisFailed, messaging.ReasonPayload{Reason: "vehicle not delivered"})

	assert.Equal(t, valueobject.OrderStatusCancelled, fixture.currentOrder(t).Status())
	assert.Equal(t, valueobject.SagaStatusCompensated, fixture.currentSaga(t).Status())
}

func TestSaga_ShouldIgnoreCompensationRepliesOutOfTurn(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	fixture.advanceToWaitingPayment(t)
	fixture.reply(t, messaging.TypePaymentFailed, messaging.PaymentOutcomePayload{Reason: "card declined"})

	fixture.reply(t, messaging.TypeWorkOrderCancelled, nil)

	current := fixture.currentSaga(t)
	assert.Equal(t, valueobject.SagaStatusCompensating, current.Status())
	assert.Equal(t, []string{messaging.TypeCancelBudget, messaging.TypeCancelWorkOrder}, current.PendingCompensations())
}

func TestSaga_ShouldIgnoreDuplicatedForwardReply(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	fixture.advanceToWaitingApproval(t)

	fixture.reply(t, messaging.TypeDiagnosisCompleted, messaging.DiagnosisCompletedPayload{})
	fixture.relayAll(t)

	budgetCommands := 0
	for _, messageType := range fixture.publisher.types() {
		if messageType == messaging.TypeGenerateBudget {
			budgetCommands++
		}
	}
	assert.Equal(t, 1, budgetCommands)
}

func TestSaga_ShouldTolerateRepliesArrivingOutOfOrder(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)

	fixture.reply(t, messaging.TypeDiagnosisCompleted, messaging.DiagnosisCompletedPayload{
		Services: []messaging.ServiceLine{{ServiceID: uuid.NewString(), Name: "Oil change", LaborPrice: 120}},
	})
	fixture.reply(t, messaging.TypeDiagnosisStarted, messaging.WorkOrderPayload{})

	assert.Equal(t, valueobject.OrderStatusInDiagnosis, fixture.currentOrder(t).Status())
	assert.Equal(t, valueobject.SagaStepBudgetApproval, fixture.currentSaga(t).Step())
}

func TestSaga_ShouldCompensateWhenOperatorCancelsDuringPayment(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	fixture.advanceToWaitingPayment(t)

	cancelled, err := serviceorder.NewCancelServiceOrderUseCase(fixture.orchestrator).Execute(fixture.ctx, serviceorder.CancelServiceOrderInput{
		OrderID: fixture.order.ID(), Reason: "customer gave up",
	})
	require.NoError(t, err)

	current := fixture.currentSaga(t)
	assert.Equal(t, valueobject.OrderStatusCancelled, cancelled.Status())
	assert.Equal(t, []string{messaging.TypeRefundPayment, messaging.TypeCancelBudget, messaging.TypeCancelWorkOrder}, current.PendingCompensations())
}

func TestSaga_ShouldRejectCancellationAfterCompletion(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	fixture.advanceToWaitingPayment(t)
	fixture.reply(t, messaging.TypePaymentConfirmed, messaging.PaymentOutcomePayload{})
	fixture.reply(t, messaging.TypeExecutionFinished, messaging.WorkOrderPayload{})

	_, err := fixture.orchestrator.Cancel(fixture.ctx, fixture.order.ID(), "too late")

	assert.ErrorIs(t, err, entity.ErrServiceOrderInvalidTransition)
}

func TestSaga_ShouldCompensateWhenStepDeadlineExpires(t *testing.T) {
	fixture := newSagaFixture(t, time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	expired, err := fixture.orchestrator.ExpireOverdue(fixture.ctx)
	require.NoError(t, err)

	order := fixture.currentOrder(t)
	assert.Equal(t, 1, expired)
	assert.Equal(t, valueobject.OrderStatusCancelled, order.Status())
	assert.Equal(t, "saga step timed out", order.CancellationReason())
	assert.Equal(t, []string{messaging.TypeCancelWorkOrder}, fixture.currentSaga(t).PendingCompensations())
}

func TestSaga_ShouldNotExpireSagasWithinTheDeadline(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)

	expired, err := fixture.orchestrator.ExpireOverdue(fixture.ctx)

	require.NoError(t, err)
	assert.Zero(t, expired)
}

func TestSagaRepository_ShouldDetectConcurrentChange(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	stale := fixture.currentSaga(t)
	fixture.reply(t, messaging.TypeDiagnosisStarted, messaging.WorkOrderPayload{})

	stale.RecordReply(messaging.TypeDiagnosisStarted, "")
	err := fixture.sagas.Save(fixture.ctx, fixture.currentOrder(t), stale)

	assert.ErrorIs(t, err, repository.ErrSagaConflict)
}

func TestSaga_ShouldFailOnUnknownSagaOrReplyType(t *testing.T) {
	fixture := newSagaFixture(t, time.Hour)
	unknownSaga, err := messaging.NewMessage(messaging.TypeDiagnosisStarted, uuid.NewString(), fixture.order.ID(), nil)
	require.NoError(t, err)
	unknownType, err := messaging.NewMessage("SomethingElse", fixture.saga.ID(), fixture.order.ID(), nil)
	require.NoError(t, err)

	assert.ErrorIs(t, fixture.orchestrator.Handle(fixture.ctx, unknownSaga), repository.ErrSagaNotFound)
	assert.ErrorIs(t, fixture.orchestrator.Handle(fixture.ctx, unknownType), messaging.ErrUnknownMessageType)
}
