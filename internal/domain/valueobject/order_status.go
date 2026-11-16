package valueobject

import "errors"

var ErrInvalidStatusTransition = errors.New("invalid order status transition")

type OrderStatus string

const (
	OrderStatusReceived        OrderStatus = "received"
	OrderStatusInDiagnosis     OrderStatus = "in_diagnosis"
	OrderStatusWaitingApproval OrderStatus = "waiting_approval"
	OrderStatusWaitingPayment  OrderStatus = "waiting_payment"
	OrderStatusInExecution     OrderStatus = "in_execution"
	OrderStatusFinished        OrderStatus = "finished"
	OrderStatusDelivered       OrderStatus = "delivered"
	OrderStatusCancelled       OrderStatus = "cancelled"
)

var progressOrder = map[OrderStatus]int{
	OrderStatusReceived:        1,
	OrderStatusInDiagnosis:     2,
	OrderStatusWaitingApproval: 3,
	OrderStatusWaitingPayment:  4,
	OrderStatusInExecution:     5,
	OrderStatusFinished:        6,
	OrderStatusDelivered:       7,
}

var listingPriority = map[OrderStatus]int{
	OrderStatusInExecution:     1,
	OrderStatusWaitingPayment:  2,
	OrderStatusWaitingApproval: 3,
	OrderStatusInDiagnosis:     4,
	OrderStatusReceived:        5,
}

func (s OrderStatus) IsBefore(next OrderStatus) bool {
	current, currentKnown := progressOrder[s]
	target, targetKnown := progressOrder[next]
	return currentKnown && targetKnown && current < target
}

func (s OrderStatus) CanBeCancelled() bool {
	return s.IsBefore(OrderStatusFinished)
}

func (s OrderStatus) CanTransitionTo(next OrderStatus) bool {
	if next == OrderStatusCancelled {
		return s.CanBeCancelled()
	}
	if next == OrderStatusDelivered {
		return s == OrderStatusFinished
	}
	return s.IsBefore(next) && next != OrderStatusDelivered
}

func (s OrderStatus) ListingPriority() int {
	return listingPriority[s]
}

func (s OrderStatus) String() string {
	return string(s)
}
