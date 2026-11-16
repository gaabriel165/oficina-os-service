package messaging

import (
	"encoding/json"
	"time"

	domain "github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
)

type envelope struct {
	MessageID  string          `json:"message_id"`
	Type       string          `json:"type"`
	SagaID     string          `json:"saga_id"`
	OrderID    string          `json:"order_id"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

func EncodeMessage(message domain.Message) (string, error) {
	encoded, err := json.Marshal(envelope{
		MessageID:  message.ID,
		Type:       message.Type,
		SagaID:     message.SagaID,
		OrderID:    message.OrderID,
		OccurredAt: message.OccurredAt,
		Payload:    message.Payload,
	})
	if err != nil {
		return "", domain.ErrMessagePayloadInvalid
	}
	return string(encoded), nil
}

func DecodeMessage(body string) (domain.Message, error) {
	var decoded envelope
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		return domain.Message{}, domain.ErrMessagePayloadInvalid
	}
	if decoded.MessageID == "" || decoded.Type == "" {
		return domain.Message{}, domain.ErrMessagePayloadInvalid
	}
	return domain.Message{
		ID:         decoded.MessageID,
		Type:       decoded.Type,
		SagaID:     decoded.SagaID,
		OrderID:    decoded.OrderID,
		OccurredAt: decoded.OccurredAt,
		Payload:    decoded.Payload,
	}, nil
}
