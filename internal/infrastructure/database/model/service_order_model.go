package model

import (
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
)

type ServiceOrderModel struct {
	ID                 string     `gorm:"type:uuid;primaryKey;column:id"`
	CustomerID         string     `gorm:"type:uuid;not null;column:customer_id"`
	VehicleID          string     `gorm:"type:uuid;not null;column:vehicle_id"`
	Status             string     `gorm:"type:varchar(50);not null;column:status"`
	Notes              string     `gorm:"type:text;column:notes"`
	TotalAmount        float64    `gorm:"type:decimal(10,2);not null;default:0;column:total_amount"`
	BudgetID           *string    `gorm:"type:uuid;column:budget_id"`
	PaymentID          *string    `gorm:"type:uuid;column:payment_id"`
	CheckoutURL        string     `gorm:"column:checkout_url"`
	CancellationReason string     `gorm:"column:cancellation_reason"`
	StartedAt          *time.Time `gorm:"column:started_at"`
	FinishedAt         *time.Time `gorm:"column:finished_at"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (ServiceOrderModel) TableName() string { return "service_orders" }

type ServiceOrderStatusHistoryModel struct {
	ID             string    `gorm:"type:uuid;primaryKey;column:id"`
	ServiceOrderID string    `gorm:"type:uuid;not null;column:service_order_id"`
	FromStatus     string    `gorm:"column:from_status"`
	ToStatus       string    `gorm:"column:to_status"`
	Reason         string    `gorm:"column:reason"`
	OccurredAt     time.Time `gorm:"column:occurred_at"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (ServiceOrderStatusHistoryModel) TableName() string { return "service_order_status_history" }

func (m *ServiceOrderModel) ToDomain() *entity.ServiceOrder {
	return entity.RestoreServiceOrder(entity.ServiceOrderSnapshot{
		ID:                 m.ID,
		CustomerID:         m.CustomerID,
		VehicleID:          m.VehicleID,
		Status:             valueobject.OrderStatus(m.Status),
		Notes:              m.Notes,
		TotalAmount:        m.TotalAmount,
		BudgetID:           stringOrEmpty(m.BudgetID),
		PaymentID:          stringOrEmpty(m.PaymentID),
		CheckoutURL:        m.CheckoutURL,
		CancellationReason: m.CancellationReason,
		StartedAt:          m.StartedAt,
		FinishedAt:         m.FinishedAt,
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
	})
}

func ServiceOrderModelFromDomain(o *entity.ServiceOrder) *ServiceOrderModel {
	return &ServiceOrderModel{
		ID:                 o.ID(),
		CustomerID:         o.CustomerID(),
		VehicleID:          o.VehicleID(),
		Status:             o.Status().String(),
		Notes:              o.Notes(),
		TotalAmount:        o.TotalAmount(),
		BudgetID:           nilIfEmpty(o.BudgetID()),
		PaymentID:          nilIfEmpty(o.PaymentID()),
		CheckoutURL:        o.CheckoutURL(),
		CancellationReason: o.CancellationReason(),
		StartedAt:          o.StartedAt(),
		FinishedAt:         o.FinishedAt(),
		CreatedAt:          o.CreatedAt(),
		UpdatedAt:          o.UpdatedAt(),
	}
}

func StatusHistoryModelsFromDomain(o *entity.ServiceOrder) []ServiceOrderStatusHistoryModel {
	models := make([]ServiceOrderStatusHistoryModel, 0, len(o.PendingHistory()))
	for _, change := range o.PendingHistory() {
		models = append(models, ServiceOrderStatusHistoryModel{
			ID:             change.ID,
			ServiceOrderID: o.ID(),
			FromStatus:     change.From.String(),
			ToStatus:       change.To.String(),
			Reason:         change.Reason,
			OccurredAt:     change.OccurredAt,
			CreatedAt:      change.OccurredAt,
			UpdatedAt:      change.OccurredAt,
		})
	}
	return models
}

func (m ServiceOrderStatusHistoryModel) ToDomain() entity.StatusChange {
	return entity.StatusChange{
		ID:         m.ID,
		From:       valueobject.OrderStatus(m.FromStatus),
		To:         valueobject.OrderStatus(m.ToStatus),
		Reason:     m.Reason,
		OccurredAt: m.OccurredAt,
	}
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nilIfEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
