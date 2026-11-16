package serviceorder_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/application/mocks"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase/serviceorder"
	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/repository"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	ctx         = context.Background()
	errDatabase = errors.New("database unavailable")
)

type sagaSpy struct {
	started   []*entity.ServiceOrder
	cancelled []string
	err       error
}

func (s *sagaSpy) Start(_ context.Context, order *entity.ServiceOrder) (*entity.Saga, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.started = append(s.started, order)
	return entity.NewSaga(order.ID(), nil)
}

func (s *sagaSpy) Cancel(_ context.Context, orderID, reason string) (*entity.ServiceOrder, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.cancelled = append(s.cancelled, orderID+":"+reason)
	return orderAt(orderID, "customer-1", valueobject.OrderStatusCancelled, time.Now()), nil
}

type notifierSpy struct {
	notified []*entity.ServiceOrder
}

func (n *notifierSpy) Notify(order *entity.ServiceOrder) {
	n.notified = append(n.notified, order)
}

func orderAt(id, customerID string, status valueobject.OrderStatus, createdAt time.Time) *entity.ServiceOrder {
	return entity.RestoreServiceOrder(entity.ServiceOrderSnapshot{
		ID: id, CustomerID: customerID, VehicleID: "vehicle-1", Status: status, CreatedAt: createdAt, UpdatedAt: createdAt,
	})
}

func customerAndVehicle(t *testing.T) (*entity.Customer, *entity.Vehicle) {
	t.Helper()
	customer, err := entity.NewCustomer("Maria Silva", "529.982.247-25", "11999999999", "maria@example.com")
	require.NoError(t, err)
	vehicle, err := entity.NewVehicle(customer.ID(), "ABC1D23", "Fiat", "Uno", 2015)
	require.NoError(t, err)
	return customer, vehicle
}

func TestCreateServiceOrder_ShouldOpenOrderAndStartSaga(t *testing.T) {
	customerRepo := mocks.NewMockCustomerRepository(t)
	vehicleRepo := mocks.NewMockVehicleRepository(t)
	saga := &sagaSpy{}
	customer, vehicle := customerAndVehicle(t)
	customerRepo.On("FindByID", customer.ID()).Return(customer, nil)
	vehicleRepo.On("FindByID", vehicle.ID()).Return(vehicle, nil)

	order, err := serviceorder.NewCreateServiceOrderUseCase(customerRepo, vehicleRepo, saga).Execute(ctx, serviceorder.CreateServiceOrderInput{
		CustomerID: customer.ID(), VehicleID: vehicle.ID(), Notes: "engine noise",
	})

	require.NoError(t, err)
	assert.Equal(t, valueobject.OrderStatusReceived, order.Status())
	assert.Equal(t, "engine noise", order.Notes())
	require.Len(t, saga.started, 1)
	assert.Equal(t, order.ID(), saga.started[0].ID())
}

func TestCreateServiceOrder_ShouldReturnErrorWhenCustomerNotFound(t *testing.T) {
	customerRepo := mocks.NewMockCustomerRepository(t)
	vehicleRepo := mocks.NewMockVehicleRepository(t)
	customerRepo.On("FindByID", "missing").Return(nil, repository.ErrCustomerNotFound)

	_, err := serviceorder.NewCreateServiceOrderUseCase(customerRepo, vehicleRepo, &sagaSpy{}).Execute(ctx, serviceorder.CreateServiceOrderInput{
		CustomerID: "missing", VehicleID: "vehicle-1",
	})

	assert.ErrorIs(t, err, repository.ErrCustomerNotFound)
}

func TestCreateServiceOrder_ShouldReturnErrorWhenVehicleNotFound(t *testing.T) {
	customerRepo := mocks.NewMockCustomerRepository(t)
	vehicleRepo := mocks.NewMockVehicleRepository(t)
	customer, _ := customerAndVehicle(t)
	customerRepo.On("FindByID", customer.ID()).Return(customer, nil)
	vehicleRepo.On("FindByID", "missing").Return(nil, repository.ErrVehicleNotFound)

	_, err := serviceorder.NewCreateServiceOrderUseCase(customerRepo, vehicleRepo, &sagaSpy{}).Execute(ctx, serviceorder.CreateServiceOrderInput{
		CustomerID: customer.ID(), VehicleID: "missing",
	})

	assert.ErrorIs(t, err, repository.ErrVehicleNotFound)
}

func TestCreateServiceOrder_ShouldRejectVehicleOfAnotherCustomer(t *testing.T) {
	customerRepo := mocks.NewMockCustomerRepository(t)
	vehicleRepo := mocks.NewMockVehicleRepository(t)
	saga := &sagaSpy{}
	customer, _ := customerAndVehicle(t)
	otherVehicle, err := entity.NewVehicle("customer-2", "ABC1D23", "Fiat", "Uno", 2015)
	require.NoError(t, err)
	customerRepo.On("FindByID", customer.ID()).Return(customer, nil)
	vehicleRepo.On("FindByID", otherVehicle.ID()).Return(otherVehicle, nil)

	_, err = serviceorder.NewCreateServiceOrderUseCase(customerRepo, vehicleRepo, saga).Execute(ctx, serviceorder.CreateServiceOrderInput{
		CustomerID: customer.ID(), VehicleID: otherVehicle.ID(),
	})

	assert.ErrorIs(t, err, entity.ErrServiceOrderVehicleNotOwnedByCustomer)
	assert.Empty(t, saga.started)
}

func TestCreateServiceOrder_ShouldReturnErrorWhenSagaCannotStart(t *testing.T) {
	customerRepo := mocks.NewMockCustomerRepository(t)
	vehicleRepo := mocks.NewMockVehicleRepository(t)
	customer, vehicle := customerAndVehicle(t)
	customerRepo.On("FindByID", customer.ID()).Return(customer, nil)
	vehicleRepo.On("FindByID", vehicle.ID()).Return(vehicle, nil)

	_, err := serviceorder.NewCreateServiceOrderUseCase(customerRepo, vehicleRepo, &sagaSpy{err: errDatabase}).Execute(ctx, serviceorder.CreateServiceOrderInput{
		CustomerID: customer.ID(), VehicleID: vehicle.ID(),
	})

	assert.ErrorIs(t, err, errDatabase)
}

func TestCancelServiceOrder_ShouldDelegateCancellationToSaga(t *testing.T) {
	saga := &sagaSpy{}

	order, err := serviceorder.NewCancelServiceOrderUseCase(saga).Execute(ctx, serviceorder.CancelServiceOrderInput{
		OrderID: "order-1", Reason: "customer gave up",
	})

	require.NoError(t, err)
	assert.True(t, order.IsCancelled())
	assert.Equal(t, []string{"order-1:customer gave up"}, saga.cancelled)
}

func TestCancelServiceOrder_ShouldRequireReason(t *testing.T) {
	saga := &sagaSpy{}

	_, err := serviceorder.NewCancelServiceOrderUseCase(saga).Execute(ctx, serviceorder.CancelServiceOrderInput{OrderID: "order-1"})

	assert.ErrorIs(t, err, entity.ErrServiceOrderCancellationReasonRequired)
	assert.Empty(t, saga.cancelled)
}

func TestDeliverVehicle_ShouldDeliverFinishedOrderAndNotifyCustomer(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	notifier := &notifierSpy{}
	order := orderAt("order-1", "customer-1", valueobject.OrderStatusFinished, time.Now())
	orderRepo.On("FindByID", ctx, "order-1").Return(order, nil)
	orderRepo.On("Update", ctx, order).Return(nil)

	result, err := serviceorder.NewDeliverVehicleUseCase(orderRepo, notifier).Execute(ctx, "order-1")

	require.NoError(t, err)
	assert.Equal(t, valueobject.OrderStatusDelivered, result.Status())
	assert.Len(t, notifier.notified, 1)
}

func TestDeliverVehicle_ShouldReturnErrorWhenOrderIsNotFinished(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	notifier := &notifierSpy{}
	orderRepo.On("FindByID", ctx, "order-1").Return(orderAt("order-1", "customer-1", valueobject.OrderStatusInExecution, time.Now()), nil)

	_, err := serviceorder.NewDeliverVehicleUseCase(orderRepo, notifier).Execute(ctx, "order-1")

	assert.ErrorIs(t, err, entity.ErrServiceOrderInvalidTransition)
	assert.Empty(t, notifier.notified)
}

func TestDeliverVehicle_ShouldReturnErrorWhenNotFound(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByID", ctx, "missing").Return(nil, repository.ErrServiceOrderNotFound)

	_, err := serviceorder.NewDeliverVehicleUseCase(orderRepo, &notifierSpy{}).Execute(ctx, "missing")

	assert.ErrorIs(t, err, repository.ErrServiceOrderNotFound)
}

func TestDeliverVehicle_ShouldNotNotifyWhenSaveFails(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	notifier := &notifierSpy{}
	order := orderAt("order-1", "customer-1", valueobject.OrderStatusFinished, time.Now())
	orderRepo.On("FindByID", ctx, "order-1").Return(order, nil)
	orderRepo.On("Update", ctx, order).Return(errDatabase)

	_, err := serviceorder.NewDeliverVehicleUseCase(orderRepo, notifier).Execute(ctx, "order-1")

	assert.ErrorIs(t, err, errDatabase)
	assert.Empty(t, notifier.notified)
}

func TestGetServiceOrder_ShouldReturnOrder(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByID", ctx, "order-1").Return(orderAt("order-1", "customer-1", valueobject.OrderStatusReceived, time.Now()), nil)

	order, err := serviceorder.NewGetServiceOrderUseCase(orderRepo).Execute(ctx, "order-1")

	require.NoError(t, err)
	assert.Equal(t, "order-1", order.ID())
}

func TestGetServiceOrderForCustomer_ShouldReturnOwnOrder(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByID", ctx, "order-1").Return(orderAt("order-1", "customer-1", valueobject.OrderStatusReceived, time.Now()), nil)

	order, err := serviceorder.NewGetServiceOrderUseCase(orderRepo).ExecuteForCustomer(ctx, "order-1", "customer-1")

	require.NoError(t, err)
	assert.Equal(t, "order-1", order.ID())
}

func TestGetServiceOrderForCustomer_ShouldForbidOrderOfAnotherCustomer(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByID", ctx, "order-1").Return(orderAt("order-1", "customer-1", valueobject.OrderStatusReceived, time.Now()), nil)

	_, err := serviceorder.NewGetServiceOrderUseCase(orderRepo).ExecuteForCustomer(ctx, "order-1", "customer-2")

	assert.ErrorIs(t, err, usecase.ErrForbidden)
}

func TestGetServiceOrderForCustomer_ShouldReturnErrorWhenNotFound(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByID", ctx, "missing").Return(nil, repository.ErrServiceOrderNotFound)

	_, err := serviceorder.NewGetServiceOrderUseCase(orderRepo).ExecuteForCustomer(ctx, "missing", "customer-1")

	assert.ErrorIs(t, err, repository.ErrServiceOrderNotFound)
}

func activeOrders() []*entity.ServiceOrder {
	morning := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	return []*entity.ServiceOrder{
		orderAt("received-new", "customer-1", valueobject.OrderStatusReceived, morning.Add(time.Hour)),
		orderAt("received-old", "customer-2", valueobject.OrderStatusReceived, morning),
		orderAt("in-execution", "customer-1", valueobject.OrderStatusInExecution, morning.Add(2*time.Hour)),
		orderAt("waiting-approval", "customer-2", valueobject.OrderStatusWaitingApproval, morning),
	}
}

func orderIDs(orders []*entity.ServiceOrder) []string {
	ids := []string{}
	for _, order := range orders {
		ids = append(ids, order.ID())
	}
	return ids
}

func TestListServiceOrders_ShouldSortByUrgencyThenAge(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByStatuses", ctx, mock.AnythingOfType("[]valueobject.OrderStatus")).Return(activeOrders(), nil)

	orders, err := serviceorder.NewListServiceOrdersUseCase(orderRepo).Execute(ctx)

	require.NoError(t, err)
	assert.Equal(t, []string{"in-execution", "waiting-approval", "received-old", "received-new"}, orderIDs(orders))
}

func TestListServiceOrders_ShouldOnlyAskForActiveStatuses(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByStatuses", ctx, mock.MatchedBy(func(statuses []valueobject.OrderStatus) bool {
		for _, status := range statuses {
			if status == valueobject.OrderStatusFinished || status == valueobject.OrderStatusDelivered || status == valueobject.OrderStatusCancelled {
				return false
			}
		}
		return len(statuses) == 5
	})).Return([]*entity.ServiceOrder{}, nil)

	orders, err := serviceorder.NewListServiceOrdersUseCase(orderRepo).Execute(ctx)

	require.NoError(t, err)
	assert.Empty(t, orders)
}

func TestListServiceOrders_ShouldReturnErrorWhenLookupFails(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByStatuses", ctx, mock.Anything).Return(nil, errDatabase)

	_, err := serviceorder.NewListServiceOrdersUseCase(orderRepo).Execute(ctx)

	assert.ErrorIs(t, err, errDatabase)
}

func TestListServiceOrdersForCustomer_ShouldReturnOnlyOwnOrders(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByStatuses", ctx, mock.Anything).Return(activeOrders(), nil)

	orders, err := serviceorder.NewListServiceOrdersUseCase(orderRepo).ExecuteForCustomer(ctx, "customer-1")

	require.NoError(t, err)
	assert.Equal(t, []string{"in-execution", "received-new"}, orderIDs(orders))
}

func TestListServiceOrdersForCustomer_ShouldReturnErrorWhenLookupFails(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByStatuses", ctx, mock.Anything).Return(nil, errDatabase)

	_, err := serviceorder.NewListServiceOrdersUseCase(orderRepo).ExecuteForCustomer(ctx, "customer-1")

	assert.ErrorIs(t, err, errDatabase)
}

var (
	operatorRequester = serviceorder.TrackingRequester{ID: "operator-1"}
	ownerRequester    = serviceorder.TrackingRequester{ID: "customer-1", IsCustomer: true}
	strangerRequester = serviceorder.TrackingRequester{ID: "customer-2", IsCustomer: true}
)

func TestGetServiceOrderHistory_ShouldReturnStatusChanges(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByID", ctx, "order-1").Return(orderAt("order-1", "customer-1", valueobject.OrderStatusInDiagnosis, time.Now()), nil)
	orderRepo.On("FindHistory", ctx, "order-1").Return([]entity.StatusChange{
		{To: valueobject.OrderStatusReceived}, {From: valueobject.OrderStatusReceived, To: valueobject.OrderStatusInDiagnosis},
	}, nil)

	history, err := serviceorder.NewGetServiceOrderHistoryUseCase(orderRepo).Execute(ctx, "order-1", ownerRequester)

	require.NoError(t, err)
	assert.Len(t, history, 2)
}

func TestGetServiceOrderHistory_ShouldForbidAnotherCustomer(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByID", ctx, "order-1").Return(orderAt("order-1", "customer-1", valueobject.OrderStatusInDiagnosis, time.Now()), nil)

	_, err := serviceorder.NewGetServiceOrderHistoryUseCase(orderRepo).Execute(ctx, "order-1", strangerRequester)

	assert.ErrorIs(t, err, usecase.ErrForbidden)
}

func TestGetServiceOrderHistory_ShouldReturnErrorWhenNotFound(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByID", ctx, "missing").Return(nil, repository.ErrServiceOrderNotFound)

	_, err := serviceorder.NewGetServiceOrderHistoryUseCase(orderRepo).Execute(ctx, "missing", operatorRequester)

	assert.ErrorIs(t, err, repository.ErrServiceOrderNotFound)
}

func TestGetServiceOrderSaga_ShouldReturnSagaToOperator(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	sagaRepo := mocks.NewMockSagaRepository(t)
	saga, err := entity.NewSaga("order-1", nil)
	require.NoError(t, err)
	orderRepo.On("FindByID", ctx, "order-1").Return(orderAt("order-1", "customer-1", valueobject.OrderStatusInDiagnosis, time.Now()), nil)
	sagaRepo.On("FindByOrderID", ctx, "order-1").Return(saga, nil)

	result, err := serviceorder.NewGetServiceOrderSagaUseCase(orderRepo, sagaRepo).Execute(ctx, "order-1", operatorRequester)

	require.NoError(t, err)
	assert.Equal(t, saga.ID(), result.ID())
}

func TestGetServiceOrderSaga_ShouldForbidAnotherCustomer(t *testing.T) {
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	sagaRepo := mocks.NewMockSagaRepository(t)
	orderRepo.On("FindByID", ctx, "order-1").Return(orderAt("order-1", "customer-1", valueobject.OrderStatusInDiagnosis, time.Now()), nil)

	_, err := serviceorder.NewGetServiceOrderSagaUseCase(orderRepo, sagaRepo).Execute(ctx, "order-1", strangerRequester)

	assert.ErrorIs(t, err, usecase.ErrForbidden)
}
