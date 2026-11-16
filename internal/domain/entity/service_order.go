package entity

import (
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
	"github.com/google/uuid"
)

type StatusTransition struct {
	From     valueobject.OrderStatus
	To       valueobject.OrderStatus
	Duration time.Duration
}

type StatusChange struct {
	ID         string
	From       valueobject.OrderStatus
	To         valueobject.OrderStatus
	Reason     string
	OccurredAt time.Time
}

type ServiceOrder struct {
	id                 string
	customerID         string
	vehicleID          string
	status             valueobject.OrderStatus
	notes              string
	totalAmount        float64
	budgetID           string
	paymentID          string
	checkoutURL        string
	cancellationReason string
	startedAt          *time.Time
	finishedAt         *time.Time
	createdAt          time.Time
	updatedAt          time.Time
	lastTransition     *StatusTransition
	pendingHistory     []StatusChange
}

type ServiceOrderSnapshot struct {
	ID                 string
	CustomerID         string
	VehicleID          string
	Status             valueobject.OrderStatus
	Notes              string
	TotalAmount        float64
	BudgetID           string
	PaymentID          string
	CheckoutURL        string
	CancellationReason string
	StartedAt          *time.Time
	FinishedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func NewServiceOrder(customerID, vehicleID, notes string) (*ServiceOrder, error) {
	if customerID == "" {
		return nil, ErrServiceOrderCustomerRequired
	}
	if vehicleID == "" {
		return nil, ErrServiceOrderVehicleRequired
	}

	now := time.Now()
	order := &ServiceOrder{
		id:         uuid.NewString(),
		customerID: customerID,
		vehicleID:  vehicleID,
		status:     valueobject.OrderStatusReceived,
		notes:      notes,
		createdAt:  now,
		updatedAt:  now,
	}
	order.recordChange("", valueobject.OrderStatusReceived, "", now)
	return order, nil
}

func RestoreServiceOrder(snapshot ServiceOrderSnapshot) *ServiceOrder {
	return &ServiceOrder{
		id:                 snapshot.ID,
		customerID:         snapshot.CustomerID,
		vehicleID:          snapshot.VehicleID,
		status:             snapshot.Status,
		notes:              snapshot.Notes,
		totalAmount:        snapshot.TotalAmount,
		budgetID:           snapshot.BudgetID,
		paymentID:          snapshot.PaymentID,
		checkoutURL:        snapshot.CheckoutURL,
		cancellationReason: snapshot.CancellationReason,
		startedAt:          snapshot.StartedAt,
		finishedAt:         snapshot.FinishedAt,
		createdAt:          snapshot.CreatedAt,
		updatedAt:          snapshot.UpdatedAt,
	}
}

func (o *ServiceOrder) ID() string                        { return o.id }
func (o *ServiceOrder) CustomerID() string                { return o.customerID }
func (o *ServiceOrder) VehicleID() string                 { return o.vehicleID }
func (o *ServiceOrder) Status() valueobject.OrderStatus   { return o.status }
func (o *ServiceOrder) Notes() string                     { return o.notes }
func (o *ServiceOrder) TotalAmount() float64              { return o.totalAmount }
func (o *ServiceOrder) BudgetID() string                  { return o.budgetID }
func (o *ServiceOrder) PaymentID() string                 { return o.paymentID }
func (o *ServiceOrder) CheckoutURL() string               { return o.checkoutURL }
func (o *ServiceOrder) CancellationReason() string        { return o.cancellationReason }
func (o *ServiceOrder) StartedAt() *time.Time             { return o.startedAt }
func (o *ServiceOrder) FinishedAt() *time.Time            { return o.finishedAt }
func (o *ServiceOrder) CreatedAt() time.Time              { return o.createdAt }
func (o *ServiceOrder) UpdatedAt() time.Time              { return o.updatedAt }
func (o *ServiceOrder) LastTransition() *StatusTransition { return o.lastTransition }
func (o *ServiceOrder) PendingHistory() []StatusChange    { return o.pendingHistory }

func (o *ServiceOrder) IsCancelled() bool {
	return o.status == valueobject.OrderStatusCancelled
}

func (o *ServiceOrder) BelongsTo(customerID string) bool {
	return o.customerID == customerID
}

func (o *ServiceOrder) AdvanceTo(next valueobject.OrderStatus) bool {
	if !o.status.IsBefore(next) || !o.status.CanTransitionTo(next) {
		return false
	}

	now := o.transitionTo(next, "")
	if next == valueobject.OrderStatusInExecution {
		o.startedAt = &now
	}
	if next == valueobject.OrderStatusFinished {
		o.finishedAt = &now
	}
	return true
}

func (o *ServiceOrder) AttachBudget(budgetID string, totalAmount float64) {
	o.budgetID = budgetID
	o.totalAmount = totalAmount
	o.updatedAt = time.Now()
}

func (o *ServiceOrder) AttachPayment(paymentID, checkoutURL string) {
	o.paymentID = paymentID
	o.checkoutURL = checkoutURL
	o.updatedAt = time.Now()
}

func (o *ServiceOrder) Cancel(reason string) error {
	if reason == "" {
		return ErrServiceOrderCancellationReasonRequired
	}
	if !o.status.CanTransitionTo(valueobject.OrderStatusCancelled) {
		return ErrServiceOrderInvalidTransition
	}
	o.transitionTo(valueobject.OrderStatusCancelled, reason)
	o.cancellationReason = reason
	return nil
}

func (o *ServiceOrder) Deliver() error {
	if !o.status.CanTransitionTo(valueobject.OrderStatusDelivered) {
		return ErrServiceOrderInvalidTransition
	}
	o.transitionTo(valueobject.OrderStatusDelivered, "")
	return nil
}

func (o *ServiceOrder) transitionTo(next valueobject.OrderStatus, reason string) time.Time {
	now := time.Now()
	o.lastTransition = &StatusTransition{From: o.status, To: next, Duration: now.Sub(o.updatedAt)}
	o.recordChange(o.status, next, reason, now)
	o.status = next
	o.updatedAt = now
	return now
}

func (o *ServiceOrder) recordChange(from, to valueobject.OrderStatus, reason string, occurredAt time.Time) {
	o.pendingHistory = append(o.pendingHistory, StatusChange{
		ID:         uuid.NewString(),
		From:       from,
		To:         to,
		Reason:     reason,
		OccurredAt: occurredAt,
	})
}
