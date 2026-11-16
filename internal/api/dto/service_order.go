package dto

import (
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
)

type CreateServiceOrderRequest struct {
	CustomerID string `json:"customer_id" binding:"required"`
	VehicleID  string `json:"vehicle_id" binding:"required"`
	Notes      string `json:"notes"`
}

type CancelServiceOrderRequest struct {
	Reason string `json:"reason" binding:"required"`
}

type ServiceOrderResponse struct {
	ID                 string                  `json:"id"`
	CustomerID         string                  `json:"customer_id"`
	VehicleID          string                  `json:"vehicle_id"`
	Status             valueobject.OrderStatus `json:"status"`
	Notes              string                  `json:"notes"`
	TotalAmount        float64                 `json:"total_amount"`
	BudgetID           string                  `json:"budget_id,omitempty"`
	PaymentID          string                  `json:"payment_id,omitempty"`
	CheckoutURL        string                  `json:"checkout_url,omitempty"`
	CancellationReason string                  `json:"cancellation_reason,omitempty"`
	StartedAt          *time.Time              `json:"started_at"`
	FinishedAt         *time.Time              `json:"finished_at"`
	CreatedAt          time.Time               `json:"created_at"`
	UpdatedAt          time.Time               `json:"updated_at"`
}

type ServiceOrderStatusResponse struct {
	ID          string                  `json:"id"`
	Status      valueobject.OrderStatus `json:"status"`
	TotalAmount float64                 `json:"total_amount"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

type StatusChangeResponse struct {
	From       valueobject.OrderStatus `json:"from,omitempty"`
	To         valueobject.OrderStatus `json:"to"`
	Reason     string                  `json:"reason,omitempty"`
	OccurredAt time.Time               `json:"occurred_at"`
}

type SagaLogEntryResponse struct {
	Kind       string    `json:"kind"`
	Name       string    `json:"name"`
	Detail     string    `json:"detail,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

type SagaResponse struct {
	ID                   string                 `json:"id"`
	OrderID              string                 `json:"order_id"`
	Status               string                 `json:"status"`
	Step                 string                 `json:"step"`
	PendingCompensations []string               `json:"pending_compensations"`
	FailureReason        string                 `json:"failure_reason,omitempty"`
	Deadline             *time.Time             `json:"deadline,omitempty"`
	StartedAt            time.Time              `json:"started_at"`
	FinishedAt           *time.Time             `json:"finished_at,omitempty"`
	Log                  []SagaLogEntryResponse `json:"log"`
}

func ToServiceOrderStatusResponse(o *entity.ServiceOrder) ServiceOrderStatusResponse {
	return ServiceOrderStatusResponse{
		ID:          o.ID(),
		Status:      o.Status(),
		TotalAmount: o.TotalAmount(),
		UpdatedAt:   o.UpdatedAt(),
	}
}

func ToServiceOrderResponse(o *entity.ServiceOrder) ServiceOrderResponse {
	return ServiceOrderResponse{
		ID:                 o.ID(),
		CustomerID:         o.CustomerID(),
		VehicleID:          o.VehicleID(),
		Status:             o.Status(),
		Notes:              o.Notes(),
		TotalAmount:        o.TotalAmount(),
		BudgetID:           o.BudgetID(),
		PaymentID:          o.PaymentID(),
		CheckoutURL:        o.CheckoutURL(),
		CancellationReason: o.CancellationReason(),
		StartedAt:          o.StartedAt(),
		FinishedAt:         o.FinishedAt(),
		CreatedAt:          o.CreatedAt(),
		UpdatedAt:          o.UpdatedAt(),
	}
}

func ToStatusHistoryResponse(history []entity.StatusChange) []StatusChangeResponse {
	responses := make([]StatusChangeResponse, 0, len(history))
	for _, change := range history {
		responses = append(responses, StatusChangeResponse{
			From:       change.From,
			To:         change.To,
			Reason:     change.Reason,
			OccurredAt: change.OccurredAt,
		})
	}
	return responses
}

func ToSagaResponse(saga *entity.Saga) SagaResponse {
	log := make([]SagaLogEntryResponse, 0, len(saga.Log()))
	for _, entry := range saga.Log() {
		log = append(log, SagaLogEntryResponse{
			Kind:       entry.Kind.String(),
			Name:       entry.Name,
			Detail:     entry.Detail,
			OccurredAt: entry.OccurredAt,
		})
	}

	pending := saga.PendingCompensations()
	if pending == nil {
		pending = []string{}
	}

	return SagaResponse{
		ID:                   saga.ID(),
		OrderID:              saga.OrderID(),
		Status:               saga.Status().String(),
		Step:                 saga.Step().String(),
		PendingCompensations: pending,
		FailureReason:        saga.FailureReason(),
		Deadline:             saga.Deadline(),
		StartedAt:            saga.CreatedAt(),
		FinishedAt:           saga.FinishedAt(),
		Log:                  log,
	}
}
