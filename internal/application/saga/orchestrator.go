package saga

import (
	"context"
	"log/slog"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/domain/repository"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
)

const (
	eventSagaStarted       = "saga.started"
	eventSagaStepCompleted = "saga.step_completed"
	eventSagaCompensating  = "saga.compensating"
	eventSagaCompleted     = "saga.completed"
	eventSagaCompensated   = "saga.compensated"
	eventSagaReplyIgnored  = "saga.reply_ignored"

	eventServiceOrderStatusChanged = "service_order.status_changed"

	reasonStepTimedOut      = "saga step timed out"
	defaultOverdueBatchSize = 20
)

type StatusNotifier interface {
	Notify(order *entity.ServiceOrder)
}

type Orchestrator struct {
	sagaRepo     repository.SagaRepository
	orderRepo    repository.ServiceOrderRepository
	customerRepo repository.CustomerRepository
	notifier     StatusNotifier
	stepTimeout  time.Duration
}

func NewOrchestrator(
	sagaRepo repository.SagaRepository,
	orderRepo repository.ServiceOrderRepository,
	customerRepo repository.CustomerRepository,
	notifier StatusNotifier,
	stepTimeout time.Duration,
) *Orchestrator {
	return &Orchestrator{
		sagaRepo:     sagaRepo,
		orderRepo:    orderRepo,
		customerRepo: customerRepo,
		notifier:     notifier,
		stepTimeout:  stepTimeout,
	}
}

func (o *Orchestrator) Start(ctx context.Context, order *entity.ServiceOrder) (*entity.Saga, error) {
	saga, err := entity.NewSaga(order.ID(), o.nextDeadline())
	if err != nil {
		return nil, err
	}

	requestDiagnosis, err := command(saga, messaging.TypeRequestDiagnosis, messaging.RequestDiagnosisPayload{
		CustomerID: order.CustomerID(),
		VehicleID:  order.VehicleID(),
		Notes:      order.Notes(),
	})
	if err != nil {
		return nil, err
	}
	saga.RecordCommand(messaging.TypeRequestDiagnosis)

	if err := o.sagaRepo.Create(ctx, order, saga, requestDiagnosis); err != nil {
		return nil, err
	}

	logSaga(ctx, eventSagaStarted, saga)
	return saga, nil
}

func (o *Orchestrator) Handle(ctx context.Context, reply messaging.Message) error {
	saga, err := o.sagaRepo.FindByID(ctx, reply.SagaID)
	if err != nil {
		return err
	}

	order, err := o.orderRepo.FindByID(ctx, saga.OrderID())
	if err != nil {
		return err
	}

	transition, known := transitions[reply.Type]
	if !known {
		return messaging.ErrUnknownMessageType
	}

	previousStatus := order.Status()
	outgoing, applied, err := transition(o, ctx, saga, order, reply)
	if err != nil {
		return err
	}
	if !applied {
		logSaga(ctx, eventSagaReplyIgnored, saga, slog.String("reply", reply.Type))
		return nil
	}

	return o.persist(ctx, saga, order, previousStatus, outgoing, slog.String("reply", reply.Type))
}

func (o *Orchestrator) Cancel(ctx context.Context, orderID, reason string) (*entity.ServiceOrder, error) {
	order, err := o.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	saga, err := o.sagaRepo.FindByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	previousStatus := order.Status()
	outgoing, err := o.fail(saga, order, reason, compensationsForStep(saga.Step()))
	if err != nil {
		return nil, err
	}

	if err := o.persist(ctx, saga, order, previousStatus, outgoing, slog.String("trigger", "cancel")); err != nil {
		return nil, err
	}
	return order, nil
}

func (o *Orchestrator) ExpireOverdue(ctx context.Context) (int, error) {
	overdue, err := o.sagaRepo.FindOverdue(ctx, time.Now(), defaultOverdueBatchSize)
	if err != nil {
		return 0, err
	}

	expired := 0
	for _, saga := range overdue {
		if _, err := o.Cancel(ctx, saga.OrderID(), reasonStepTimedOut); err != nil {
			return expired, err
		}
		expired++
	}
	return expired, nil
}

func (o *Orchestrator) persist(
	ctx context.Context,
	saga *entity.Saga,
	order *entity.ServiceOrder,
	previousStatus valueobject.OrderStatus,
	outgoing []messaging.OutgoingMessage,
	attrs ...any,
) error {
	if err := o.sagaRepo.Save(ctx, order, saga, outgoing...); err != nil {
		return err
	}

	logSaga(ctx, eventForStatus(saga.Status()), saga, attrs...)
	if order.Status() != previousStatus {
		logStatusChange(ctx, saga, order)
		o.notifier.Notify(order)
	}
	return nil
}

func (o *Orchestrator) fail(
	saga *entity.Saga,
	order *entity.ServiceOrder,
	reason string,
	compensations []string,
) ([]messaging.OutgoingMessage, error) {
	if err := order.Cancel(reason); err != nil {
		return nil, err
	}

	firstCompensation, err := saga.Fail(reason, compensations)
	if err != nil {
		return nil, err
	}
	return compensationMessages(saga, firstCompensation)
}

func (o *Orchestrator) nextDeadline() *time.Time {
	if o.stepTimeout <= 0 {
		return nil
	}
	deadline := time.Now().Add(o.stepTimeout)
	return &deadline
}

func compensationMessages(saga *entity.Saga, commandType string) ([]messaging.OutgoingMessage, error) {
	if commandType == "" {
		return nil, nil
	}
	outgoing, err := compensationCommand(saga, commandType)
	if err != nil {
		return nil, err
	}
	return []messaging.OutgoingMessage{outgoing}, nil
}

func compensationsForStep(step valueobject.SagaStep) []string {
	switch step {
	case valueobject.SagaStepDiagnosis:
		return []string{messaging.TypeCancelWorkOrder}
	case valueobject.SagaStepBudgetApproval:
		return []string{messaging.TypeCancelBudget, messaging.TypeCancelWorkOrder}
	default:
		return []string{messaging.TypeRefundPayment, messaging.TypeCancelBudget, messaging.TypeCancelWorkOrder}
	}
}

func eventForStatus(status valueobject.SagaStatus) string {
	switch status {
	case valueobject.SagaStatusCompensating:
		return eventSagaCompensating
	case valueobject.SagaStatusCompleted:
		return eventSagaCompleted
	case valueobject.SagaStatusCompensated:
		return eventSagaCompensated
	default:
		return eventSagaStepCompleted
	}
}

func logStatusChange(ctx context.Context, saga *entity.Saga, order *entity.ServiceOrder) {
	fields := []any{
		slog.String("event", eventServiceOrderStatusChanged),
		slog.String("order_id", order.ID()),
		slog.String("saga_id", saga.ID()),
		slog.String("status", order.Status().String()),
		slog.Float64("total_amount", order.TotalAmount()),
	}
	if transition := order.LastTransition(); transition != nil {
		fields = append(fields,
			slog.String("from_status", transition.From.String()),
			slog.String("to_status", transition.To.String()),
			slog.Float64("duration_minutes", transition.Duration.Minutes()),
		)
	}
	slog.InfoContext(ctx, eventServiceOrderStatusChanged, fields...)
}

func logSaga(ctx context.Context, event string, saga *entity.Saga, attrs ...any) {
	fields := append([]any{
		slog.String("event", event),
		slog.String("saga_id", saga.ID()),
		slog.String("order_id", saga.OrderID()),
		slog.String("saga_status", saga.Status().String()),
		slog.String("saga_step", saga.Step().String()),
	}, attrs...)
	if saga.FailureReason() != "" {
		fields = append(fields, slog.String("failure_reason", saga.FailureReason()))
	}
	slog.InfoContext(ctx, event, fields...)
}
