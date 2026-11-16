package messaging_test

import (
	"testing"

	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/stretchr/testify/assert"
)

func TestNewMessage_ShouldBuildMessageWithEncodedPayload(t *testing.T) {
	message, err := messaging.NewMessage(messaging.TypeCancelBudget, "saga-1", "order-1", messaging.ReasonPayload{Reason: "rejected"})

	assert.NoError(t, err)
	assert.NotEmpty(t, message.ID)
	assert.Equal(t, messaging.TypeCancelBudget, message.Type)
	assert.Equal(t, "saga-1", message.SagaID)
	assert.Equal(t, "order-1", message.OrderID)
	assert.False(t, message.OccurredAt.IsZero())
	assert.JSONEq(t, `{"reason":"rejected"}`, string(message.Payload))
}

func TestNewMessage_ShouldUseEmptyObjectWhenPayloadIsNil(t *testing.T) {
	message, err := messaging.NewMessage(messaging.TypeRequestPayment, "saga-1", "order-1", nil)

	assert.NoError(t, err)
	assert.JSONEq(t, `{}`, string(message.Payload))
}

func TestNewMessage_ShouldReturnErrorWhenTypeIsEmpty(t *testing.T) {
	_, err := messaging.NewMessage("", "saga-1", "order-1", nil)

	assert.ErrorIs(t, err, messaging.ErrMessageTypeRequired)
}

func TestNewMessage_ShouldReturnErrorWhenSagaIsEmpty(t *testing.T) {
	_, err := messaging.NewMessage(messaging.TypeRequestPayment, "", "order-1", nil)

	assert.ErrorIs(t, err, messaging.ErrMessageSagaRequired)
}

func TestNewMessage_ShouldReturnErrorWhenOrderIsEmpty(t *testing.T) {
	_, err := messaging.NewMessage(messaging.TypeRequestPayment, "saga-1", "", nil)

	assert.ErrorIs(t, err, messaging.ErrMessageOrderRequired)
}

func TestNewMessage_ShouldReturnErrorWhenPayloadCannotBeEncoded(t *testing.T) {
	_, err := messaging.NewMessage(messaging.TypeRequestPayment, "saga-1", "order-1", make(chan int))

	assert.ErrorIs(t, err, messaging.ErrMessagePayloadInvalid)
}

func TestDecodePayload_ShouldFillTarget(t *testing.T) {
	message, err := messaging.NewMessage(messaging.TypeCancelBudget, "saga-1", "order-1", messaging.ReasonPayload{Reason: "rejected"})
	assert.NoError(t, err)

	var payload messaging.ReasonPayload
	err = message.DecodePayload(&payload)

	assert.NoError(t, err)
	assert.Equal(t, "rejected", payload.Reason)
}

func TestDecodePayload_ShouldReturnErrorWhenPayloadIsMalformed(t *testing.T) {
	message := messaging.Message{Payload: []byte("not-json")}

	var payload messaging.ReasonPayload
	err := message.DecodePayload(&payload)

	assert.ErrorIs(t, err, messaging.ErrMessagePayloadInvalid)
}

func TestDecodePayload_ShouldIgnoreEmptyPayload(t *testing.T) {
	var payload messaging.ReasonPayload
	err := messaging.Message{}.DecodePayload(&payload)

	assert.NoError(t, err)
	assert.Empty(t, payload.Reason)
}
