package saga_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/application/mocks"
	"github.com/gabrielcamargo/oficina-api/internal/application/saga"
	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/domain/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	ctx         = context.Background()
	errDatabase = errors.New("database unavailable")
)

type statusNotifierSpy struct {
	notified []string
}

func (s *statusNotifierSpy) Notify(order *entity.ServiceOrder) {
	s.notified = append(s.notified, order.Status().String())
}

type sagaStore struct {
	saga      *entity.Saga
	order     *entity.ServiceOrder
	published []messaging.OutgoingMessage
	saveErr   error
	overdue   []*entity.Saga
}

func (s *sagaStore) Create(_ context.Context, order *entity.ServiceOrder, saga *entity.Saga, outgoing ...messaging.OutgoingMessage) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.order = order
	s.saga = saga
	s.published = append(s.published, outgoing...)
	return nil
}

func (s *sagaStore) Save(_ context.Context, _ *entity.ServiceOrder, _ *entity.Saga, outgoing ...messaging.OutgoingMessage) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.published = append(s.published, outgoing...)
	return nil
}

func (s *sagaStore) FindByID(_ context.Context, id string) (*entity.Saga, error) {
	if s.saga == nil || s.saga.ID() != id {
		return nil, repository.ErrSagaNotFound
	}
	return s.saga, nil
}

func (s *sagaStore) FindByOrderID(_ context.Context, orderID string) (*entity.Saga, error) {
	if s.saga == nil || s.saga.OrderID() != orderID {
		return nil, repository.ErrSagaNotFound
	}
	return s.saga, nil
}

func (s *sagaStore) FindOverdue(_ context.Context, _ time.Time, _ int) ([]*entity.Saga, error) {
	return s.overdue, nil
}

func (s *sagaStore) publishedTypes() []string {
	types := []string{}
	for _, outgoing := range s.published {
		types = append(types, outgoing.Message.Type)
	}
	return types
}

func (s *sagaStore) lastPublished(t *testing.T) messaging.OutgoingMessage {
	t.Helper()
	require.NotEmpty(t, s.published)
	return s.published[len(s.published)-1]
}

type scenario struct {
	store        *sagaStore
	orderRepo    *mocks.MockServiceOrderRepository
	customerRepo *mocks.MockCustomerRepository
	notifier     *statusNotifierSpy
	orchestrator *saga.Orchestrator
	order        *entity.ServiceOrder
	customer     *entity.Customer
}

func newScenario(t *testing.T) *scenario {
	t.Helper()
	customer, err := entity.NewCustomer("Maria Silva", "529.982.247-25", "11999999999", "maria@example.com")
	require.NoError(t, err)
	order, err := entity.NewServiceOrder(customer.ID(), "vehicle-1", "engine noise")
	require.NoError(t, err)

	s := &scenario{
		store:        &sagaStore{},
		orderRepo:    mocks.NewMockServiceOrderRepository(t),
		customerRepo: mocks.NewMockCustomerRepository(t),
		notifier:     &statusNotifierSpy{},
		order:        order,
		customer:     customer,
	}
	s.orderRepo.On("FindByID", mock.Anything, order.ID()).Return(order, nil).Maybe()
	s.customerRepo.On("FindByID", customer.ID()).Return(customer, nil).Maybe()
	s.orchestrator = saga.NewOrchestrator(s.store, s.orderRepo, s.customerRepo, s.notifier, time.Hour)
	return s
}

func startedScenario(t *testing.T) *scenario {
	t.Helper()
	s := newScenario(t)
	_, err := s.orchestrator.Start(ctx, s.order)
	require.NoError(t, err)
	return s
}

func (s *scenario) reply(t *testing.T, messageType string, payload any) error {
	t.Helper()
	message, err := messaging.NewMessage(messageType, s.store.saga.ID(), s.order.ID(), payload)
	require.NoError(t, err)
	return s.orchestrator.Handle(ctx, message)
}

func (s *scenario) mustReply(t *testing.T, messageType string, payload any) {
	t.Helper()
	require.NoError(t, s.reply(t, messageType, payload))
}

var diagnosis = messaging.DiagnosisCompletedPayload{
	WorkOrderID: "order-1",
	Services:    []messaging.ServiceLine{{ServiceID: "service-1", Name: "Troca de óleo", LaborPrice: 120}},
	Parts:       []messaging.PartLine{{PartID: "part-1", Name: "Filtro de óleo", Quantity: 2, UnitPrice: 30}},
}

func (s *scenario) untilWaitingApproval(t *testing.T) {
	t.Helper()
	s.mustReply(t, messaging.TypeDiagnosisStarted, nil)
	s.mustReply(t, messaging.TypeDiagnosisCompleted, diagnosis)
	s.mustReply(t, messaging.TypeBudgetGenerated, messaging.BudgetGeneratedPayload{BudgetID: "budget-1", TotalAmount: 180})
}

func (s *scenario) untilWaitingPayment(t *testing.T) {
	t.Helper()
	s.untilWaitingApproval(t)
	s.mustReply(t, messaging.TypeBudgetApproved, nil)
	s.mustReply(t, messaging.TypePaymentRequested, messaging.PaymentRequestedPayload{PaymentID: "payment-1", CheckoutURL: "https://checkout.example.com/pref-1"})
}

func (s *scenario) untilExecutionRequested(t *testing.T) {
	t.Helper()
	s.untilWaitingPayment(t)
	s.mustReply(t, messaging.TypePaymentConfirmed, nil)
}
