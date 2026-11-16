package saga

import (
	"context"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
)

type transitionFunc func(
	o *Orchestrator,
	ctx context.Context,
	saga *entity.Saga,
	order *entity.ServiceOrder,
	reply messaging.Message,
) ([]messaging.OutgoingMessage, bool, error)

var transitions = map[string]transitionFunc{
	messaging.TypeDiagnosisStarted:   onDiagnosisStarted,
	messaging.TypeDiagnosisCompleted: onDiagnosisCompleted,
	messaging.TypeDiagnosisFailed:    failWith(),
	messaging.TypeBudgetGenerated:    onBudgetGenerated,
	messaging.TypeBudgetApproved:     onBudgetApproved,
	messaging.TypeBudgetRejected:     failWith(messaging.TypeCancelWorkOrder),
	messaging.TypePaymentRequested:   onPaymentRequested,
	messaging.TypePaymentConfirmed:   onPaymentConfirmed,
	messaging.TypePaymentFailed:      failWith(messaging.TypeCancelBudget, messaging.TypeCancelWorkOrder),
	messaging.TypeExecutionStarted:   onExecutionStarted,
	messaging.TypeExecutionRejected:  failWith(messaging.TypeRefundPayment, messaging.TypeCancelBudget, messaging.TypeCancelWorkOrder),
	messaging.TypeExecutionFinished:  onExecutionFinished,
	messaging.TypeWorkOrderCancelled: onCompensationConfirmed,
	messaging.TypeBudgetCancelled:    onCompensationConfirmed,
	messaging.TypePaymentRefunded:    onCompensationConfirmed,
}

func onDiagnosisStarted(_ *Orchestrator, _ context.Context, saga *entity.Saga, order *entity.ServiceOrder, reply messaging.Message) ([]messaging.OutgoingMessage, bool, error) {
	if !saga.IsRunning() {
		return nil, false, nil
	}
	saga.RecordReply(reply.Type, "")
	order.AdvanceTo(valueobject.OrderStatusInDiagnosis)
	return nil, true, nil
}

func onDiagnosisCompleted(o *Orchestrator, _ context.Context, saga *entity.Saga, order *entity.ServiceOrder, reply messaging.Message) ([]messaging.OutgoingMessage, bool, error) {
	if !saga.IsAt(valueobject.SagaStepDiagnosis) {
		return nil, false, nil
	}

	var diagnosis messaging.DiagnosisCompletedPayload
	if err := reply.DecodePayload(&diagnosis); err != nil {
		return nil, false, err
	}

	customer, err := o.customerRepo.FindByID(order.CustomerID())
	if err != nil {
		return nil, false, err
	}

	saga.RecordReply(reply.Type, "")
	order.AdvanceTo(valueobject.OrderStatusInDiagnosis)
	return advance(o, saga, valueobject.SagaStepBudgetApproval, messaging.TypeGenerateBudget, messaging.GenerateBudgetPayload{
		CustomerID:    customer.ID(),
		CustomerName:  customer.Name(),
		CustomerEmail: customer.Email(),
		Services:      diagnosis.Services,
		Parts:         diagnosis.Parts,
	})
}

func onBudgetGenerated(_ *Orchestrator, _ context.Context, saga *entity.Saga, order *entity.ServiceOrder, reply messaging.Message) ([]messaging.OutgoingMessage, bool, error) {
	if !saga.IsRunning() {
		return nil, false, nil
	}

	var budget messaging.BudgetGeneratedPayload
	if err := reply.DecodePayload(&budget); err != nil {
		return nil, false, err
	}

	saga.RecordReply(reply.Type, "")
	order.AttachBudget(budget.BudgetID, budget.TotalAmount)
	order.AdvanceTo(valueobject.OrderStatusWaitingApproval)
	return nil, true, nil
}

func onBudgetApproved(o *Orchestrator, _ context.Context, saga *entity.Saga, _ *entity.ServiceOrder, reply messaging.Message) ([]messaging.OutgoingMessage, bool, error) {
	if !saga.IsAt(valueobject.SagaStepBudgetApproval) {
		return nil, false, nil
	}
	saga.RecordReply(reply.Type, "")
	return advance(o, saga, valueobject.SagaStepPayment, messaging.TypeRequestPayment, nil)
}

func onPaymentRequested(_ *Orchestrator, _ context.Context, saga *entity.Saga, order *entity.ServiceOrder, reply messaging.Message) ([]messaging.OutgoingMessage, bool, error) {
	if !saga.IsRunning() {
		return nil, false, nil
	}

	var payment messaging.PaymentRequestedPayload
	if err := reply.DecodePayload(&payment); err != nil {
		return nil, false, err
	}

	saga.RecordReply(reply.Type, "")
	order.AttachPayment(payment.PaymentID, payment.CheckoutURL)
	order.AdvanceTo(valueobject.OrderStatusWaitingPayment)
	return nil, true, nil
}

func onPaymentConfirmed(o *Orchestrator, _ context.Context, saga *entity.Saga, _ *entity.ServiceOrder, reply messaging.Message) ([]messaging.OutgoingMessage, bool, error) {
	if !saga.IsAt(valueobject.SagaStepPayment) {
		return nil, false, nil
	}
	saga.RecordReply(reply.Type, "")

	outgoing, err := command(saga, messaging.TypeStartExecution, nil)
	if err != nil {
		return nil, false, err
	}
	if err := saga.MoveTo(valueobject.SagaStepExecution, nil); err != nil {
		return nil, false, err
	}
	saga.RecordCommand(messaging.TypeStartExecution)
	return []messaging.OutgoingMessage{outgoing}, true, nil
}

func onExecutionStarted(_ *Orchestrator, _ context.Context, saga *entity.Saga, order *entity.ServiceOrder, reply messaging.Message) ([]messaging.OutgoingMessage, bool, error) {
	if !saga.IsRunning() {
		return nil, false, nil
	}
	saga.RecordReply(reply.Type, "")
	order.AdvanceTo(valueobject.OrderStatusInExecution)
	return nil, true, nil
}

func onExecutionFinished(_ *Orchestrator, _ context.Context, saga *entity.Saga, order *entity.ServiceOrder, reply messaging.Message) ([]messaging.OutgoingMessage, bool, error) {
	if !saga.IsRunning() {
		return nil, false, nil
	}
	saga.RecordReply(reply.Type, "")
	order.AdvanceTo(valueobject.OrderStatusFinished)
	if err := saga.Complete(); err != nil {
		return nil, false, err
	}
	return nil, true, nil
}

func onCompensationConfirmed(_ *Orchestrator, _ context.Context, saga *entity.Saga, _ *entity.ServiceOrder, reply messaging.Message) ([]messaging.OutgoingMessage, bool, error) {
	nextCompensation, confirmed := saga.ConfirmCompensation(compensationByReply[reply.Type])
	if !confirmed {
		return nil, false, nil
	}

	outgoing, err := compensationMessages(saga, nextCompensation)
	if err != nil {
		return nil, false, err
	}
	return outgoing, true, nil
}

func failWith(compensations ...string) transitionFunc {
	return func(o *Orchestrator, _ context.Context, saga *entity.Saga, order *entity.ServiceOrder, reply messaging.Message) ([]messaging.OutgoingMessage, bool, error) {
		if !saga.IsRunning() {
			return nil, false, nil
		}

		var failure messaging.ReasonPayload
		if err := reply.DecodePayload(&failure); err != nil {
			return nil, false, err
		}

		saga.RecordReply(reply.Type, failure.Reason)
		outgoing, err := o.fail(saga, order, reasonOrReplyType(failure.Reason, reply.Type), compensations)
		if err != nil {
			return nil, false, err
		}
		return outgoing, true, nil
	}
}

func advance(o *Orchestrator, saga *entity.Saga, step valueobject.SagaStep, commandType string, payload any) ([]messaging.OutgoingMessage, bool, error) {
	outgoing, err := command(saga, commandType, payload)
	if err != nil {
		return nil, false, err
	}
	if err := saga.MoveTo(step, o.nextDeadline()); err != nil {
		return nil, false, err
	}
	saga.RecordCommand(commandType)
	return []messaging.OutgoingMessage{outgoing}, true, nil
}

func reasonOrReplyType(reason, replyType string) string {
	if reason == "" {
		return replyType
	}
	return reason
}
