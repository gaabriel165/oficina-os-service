package saga

import (
	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
)

var commandDestinations = map[string]messaging.Destination{
	messaging.TypeRequestDiagnosis: messaging.DestinationExecutionCommands,
	messaging.TypeStartExecution:   messaging.DestinationExecutionCommands,
	messaging.TypeCancelWorkOrder:  messaging.DestinationExecutionCommands,
	messaging.TypeGenerateBudget:   messaging.DestinationBillingCommands,
	messaging.TypeRequestPayment:   messaging.DestinationBillingCommands,
	messaging.TypeCancelBudget:     messaging.DestinationBillingCommands,
	messaging.TypeRefundPayment:    messaging.DestinationBillingCommands,
}

var compensationByReply = map[string]string{
	messaging.TypeWorkOrderCancelled: messaging.TypeCancelWorkOrder,
	messaging.TypeBudgetCancelled:    messaging.TypeCancelBudget,
	messaging.TypePaymentRefunded:    messaging.TypeRefundPayment,
}

func command(saga *entity.Saga, commandType string, payload any) (messaging.OutgoingMessage, error) {
	destination, known := commandDestinations[commandType]
	if !known {
		return messaging.OutgoingMessage{}, messaging.ErrUnknownMessageType
	}

	message, err := messaging.NewMessage(commandType, saga.ID(), saga.OrderID(), payload)
	if err != nil {
		return messaging.OutgoingMessage{}, err
	}
	return messaging.OutgoingMessage{Destination: destination, Message: message}, nil
}

func compensationCommand(saga *entity.Saga, commandType string) (messaging.OutgoingMessage, error) {
	return command(saga, commandType, messaging.ReasonPayload{Reason: saga.FailureReason()})
}
