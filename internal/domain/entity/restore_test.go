package entity_test

import (
	"testing"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
	"github.com/stretchr/testify/assert"
)

func TestRestoreCustomer_ShouldRestoreAllFields(t *testing.T) {
	now := time.Now()
	c := entity.RestoreCustomer("id-1", "John Doe", string(entity.DocumentTypeCPF), "52998224725", "11999999999", "john@email.com", "inactive", now, now)

	assert.Equal(t, "id-1", c.ID())
	assert.Equal(t, "John Doe", c.Name())
	assert.Equal(t, "52998224725", c.Document())
	assert.Equal(t, valueobject.CustomerStatusInactive, c.Status())
	assert.Equal(t, now, c.CreatedAt())
	assert.Equal(t, now, c.UpdatedAt())
}

func TestRestoreVehicle_ShouldRestoreAllFields(t *testing.T) {
	now := time.Now()
	v := entity.RestoreVehicle("id-1", "cust-1", "ABC1234", "Toyota", "Corolla", 2020, now, now)

	assert.Equal(t, "id-1", v.ID())
	assert.Equal(t, "cust-1", v.CustomerID())
	assert.Equal(t, "ABC1234", v.Plate())
	assert.Equal(t, now, v.CreatedAt())
	assert.Equal(t, now, v.UpdatedAt())
}

func TestRestoreServiceOrder_ShouldRestoreAllFields(t *testing.T) {
	now := time.Now()
	order := entity.RestoreServiceOrder(entity.ServiceOrderSnapshot{
		ID:                 "order-1",
		CustomerID:         "cust-1",
		VehicleID:          "veh-1",
		Status:             valueobject.OrderStatusWaitingPayment,
		Notes:              "engine noise",
		TotalAmount:        179.80,
		BudgetID:           "budget-1",
		PaymentID:          "payment-1",
		CheckoutURL:        "https://pay.example/checkout",
		CancellationReason: "",
		StartedAt:          &now,
		FinishedAt:         &now,
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	assert.Equal(t, "order-1", order.ID())
	assert.Equal(t, "cust-1", order.CustomerID())
	assert.Equal(t, "veh-1", order.VehicleID())
	assert.Equal(t, valueobject.OrderStatusWaitingPayment, order.Status())
	assert.Equal(t, "engine noise", order.Notes())
	assert.Equal(t, 179.80, order.TotalAmount())
	assert.Equal(t, "budget-1", order.BudgetID())
	assert.Equal(t, "payment-1", order.PaymentID())
	assert.Equal(t, "https://pay.example/checkout", order.CheckoutURL())
	assert.Equal(t, &now, order.StartedAt())
	assert.Equal(t, &now, order.FinishedAt())
	assert.Equal(t, now, order.CreatedAt())
	assert.Equal(t, now, order.UpdatedAt())
	assert.Empty(t, order.PendingHistory())
	assert.Nil(t, order.LastTransition())
}
