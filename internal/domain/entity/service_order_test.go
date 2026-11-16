package entity_test

import (
	"testing"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newServiceOrder(t *testing.T) *entity.ServiceOrder {
	t.Helper()
	order, err := entity.NewServiceOrder("customer-1", "vehicle-1", "engine noise")
	require.NoError(t, err)
	return order
}

func serviceOrderAt(status valueobject.OrderStatus) *entity.ServiceOrder {
	return entity.RestoreServiceOrder(entity.ServiceOrderSnapshot{
		ID:         "order-1",
		CustomerID: "customer-1",
		VehicleID:  "vehicle-1",
		Status:     status,
		CreatedAt:  time.Now().Add(-time.Hour),
		UpdatedAt:  time.Now().Add(-time.Minute),
	})
}

func TestNewServiceOrder_ShouldStartReceivedWithHistory(t *testing.T) {
	order := newServiceOrder(t)

	assert.NotEmpty(t, order.ID())
	assert.Equal(t, "customer-1", order.CustomerID())
	assert.Equal(t, "vehicle-1", order.VehicleID())
	assert.Equal(t, "engine noise", order.Notes())
	assert.Equal(t, valueobject.OrderStatusReceived, order.Status())
	assert.Zero(t, order.TotalAmount())
	assert.Empty(t, order.BudgetID())
	assert.Empty(t, order.PaymentID())
	assert.Nil(t, order.StartedAt())
	assert.Nil(t, order.FinishedAt())
	assert.Nil(t, order.LastTransition())
	assert.True(t, order.BelongsTo("customer-1"))
	assert.False(t, order.BelongsTo("customer-2"))
	require.Len(t, order.PendingHistory(), 1)
	assert.Equal(t, valueobject.OrderStatusReceived, order.PendingHistory()[0].To)
}

func TestNewServiceOrder_ShouldRequireCustomer(t *testing.T) {
	_, err := entity.NewServiceOrder("", "vehicle-1", "")

	assert.ErrorIs(t, err, entity.ErrServiceOrderCustomerRequired)
}

func TestNewServiceOrder_ShouldRequireVehicle(t *testing.T) {
	_, err := entity.NewServiceOrder("customer-1", "", "")

	assert.ErrorIs(t, err, entity.ErrServiceOrderVehicleRequired)
}

func TestServiceOrderAdvanceTo_ShouldMoveForwardAndRecordTransition(t *testing.T) {
	order := newServiceOrder(t)

	advanced := order.AdvanceTo(valueobject.OrderStatusInDiagnosis)

	assert.True(t, advanced)
	assert.Equal(t, valueobject.OrderStatusInDiagnosis, order.Status())
	require.NotNil(t, order.LastTransition())
	assert.Equal(t, valueobject.OrderStatusReceived, order.LastTransition().From)
	assert.Equal(t, valueobject.OrderStatusInDiagnosis, order.LastTransition().To)
	assert.Len(t, order.PendingHistory(), 2)
}

func TestServiceOrderAdvanceTo_ShouldSkipIntermediateStatuses(t *testing.T) {
	order := newServiceOrder(t)

	advanced := order.AdvanceTo(valueobject.OrderStatusWaitingPayment)

	assert.True(t, advanced)
	assert.Equal(t, valueobject.OrderStatusWaitingPayment, order.Status())
}

func TestServiceOrderAdvanceTo_ShouldIgnoreBackwardStatus(t *testing.T) {
	order := serviceOrderAt(valueobject.OrderStatusWaitingPayment)

	advanced := order.AdvanceTo(valueobject.OrderStatusInDiagnosis)

	assert.False(t, advanced)
	assert.Equal(t, valueobject.OrderStatusWaitingPayment, order.Status())
	assert.Empty(t, order.PendingHistory())
}

func TestServiceOrderAdvanceTo_ShouldIgnoreWhenCancelled(t *testing.T) {
	order := serviceOrderAt(valueobject.OrderStatusCancelled)

	advanced := order.AdvanceTo(valueobject.OrderStatusInExecution)

	assert.False(t, advanced)
	assert.True(t, order.IsCancelled())
}

func TestServiceOrderAdvanceTo_ShouldNotDeliverDirectly(t *testing.T) {
	order := serviceOrderAt(valueobject.OrderStatusInExecution)

	advanced := order.AdvanceTo(valueobject.OrderStatusDelivered)

	assert.False(t, advanced)
}

func TestServiceOrderAdvanceTo_ShouldRecordExecutionStartAndFinish(t *testing.T) {
	order := serviceOrderAt(valueobject.OrderStatusWaitingPayment)

	order.AdvanceTo(valueobject.OrderStatusInExecution)
	order.AdvanceTo(valueobject.OrderStatusFinished)

	assert.NotNil(t, order.StartedAt())
	assert.NotNil(t, order.FinishedAt())
}

func TestServiceOrderAttachBudget_ShouldStoreBudgetAndTotal(t *testing.T) {
	order := newServiceOrder(t)

	order.AttachBudget("budget-1", 180)

	assert.Equal(t, "budget-1", order.BudgetID())
	assert.Equal(t, 180.0, order.TotalAmount())
}

func TestServiceOrderAttachPayment_ShouldStorePaymentAndCheckout(t *testing.T) {
	order := newServiceOrder(t)

	order.AttachPayment("payment-1", "https://checkout.example.com/pref-1")

	assert.Equal(t, "payment-1", order.PaymentID())
	assert.Equal(t, "https://checkout.example.com/pref-1", order.CheckoutURL())
}

func TestServiceOrderCancel_ShouldStoreReasonInOrderAndHistory(t *testing.T) {
	order := serviceOrderAt(valueobject.OrderStatusInExecution)

	err := order.Cancel("out of stock")

	require.NoError(t, err)
	assert.True(t, order.IsCancelled())
	assert.Equal(t, "out of stock", order.CancellationReason())
	require.Len(t, order.PendingHistory(), 1)
	assert.Equal(t, "out of stock", order.PendingHistory()[0].Reason)
}

func TestServiceOrderCancel_ShouldRequireReason(t *testing.T) {
	order := newServiceOrder(t)

	err := order.Cancel("")

	assert.ErrorIs(t, err, entity.ErrServiceOrderCancellationReasonRequired)
}

func TestServiceOrderCancel_ShouldFailOnceFinished(t *testing.T) {
	for _, status := range []valueobject.OrderStatus{
		valueobject.OrderStatusFinished, valueobject.OrderStatusDelivered, valueobject.OrderStatusCancelled,
	} {
		t.Run(status.String(), func(t *testing.T) {
			err := serviceOrderAt(status).Cancel("changed my mind")

			assert.ErrorIs(t, err, entity.ErrServiceOrderInvalidTransition)
		})
	}
}

func TestServiceOrderDeliver_ShouldDeliverFinishedOrder(t *testing.T) {
	order := serviceOrderAt(valueobject.OrderStatusFinished)

	err := order.Deliver()

	require.NoError(t, err)
	assert.Equal(t, valueobject.OrderStatusDelivered, order.Status())
}

func TestServiceOrderDeliver_ShouldFailBeforeFinished(t *testing.T) {
	order := serviceOrderAt(valueobject.OrderStatusInExecution)

	err := order.Deliver()

	assert.ErrorIs(t, err, entity.ErrServiceOrderInvalidTransition)
}

func TestOrderStatus_ShouldPrioritizeOrdersInExecutionWhenListing(t *testing.T) {
	assert.Less(t, valueobject.OrderStatusInExecution.ListingPriority(), valueobject.OrderStatusWaitingPayment.ListingPriority())
	assert.Less(t, valueobject.OrderStatusWaitingApproval.ListingPriority(), valueobject.OrderStatusReceived.ListingPriority())
	assert.Zero(t, valueobject.OrderStatusDelivered.ListingPriority())
}
