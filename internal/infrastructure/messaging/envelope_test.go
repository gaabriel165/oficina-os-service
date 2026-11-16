package messaging_test

import (
	"testing"

	domain "github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/messaging"
	"github.com/stretchr/testify/assert"
)

func TestDecodeMessage_ShouldReturnErrorWhenBodyIsNotJSON(t *testing.T) {
	_, err := messaging.DecodeMessage("not-json")

	assert.ErrorIs(t, err, domain.ErrMessagePayloadInvalid)
}

func TestDecodeMessage_ShouldReturnErrorWhenIdentifierIsMissing(t *testing.T) {
	_, err := messaging.DecodeMessage(`{"type":"RequestPayment"}`)

	assert.ErrorIs(t, err, domain.ErrMessagePayloadInvalid)
}

func TestDecodeMessage_ShouldReturnErrorWhenTypeIsMissing(t *testing.T) {
	_, err := messaging.DecodeMessage(`{"message_id":"message-1"}`)

	assert.ErrorIs(t, err, domain.ErrMessagePayloadInvalid)
}

func TestEncodeMessage_ShouldReturnErrorWhenPayloadIsNotJSON(t *testing.T) {
	_, err := messaging.EncodeMessage(domain.Message{ID: "message-1", Type: "RequestPayment", Payload: []byte("not-json")})

	assert.ErrorIs(t, err, domain.ErrMessagePayloadInvalid)
}
