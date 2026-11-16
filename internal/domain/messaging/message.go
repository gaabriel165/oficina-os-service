package messaging

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Message struct {
	ID         string
	Type       string
	SagaID     string
	OrderID    string
	OccurredAt time.Time
	Payload    json.RawMessage
}

func NewMessage(messageType, sagaID, orderID string, payload any) (Message, error) {
	if messageType == "" {
		return Message{}, ErrMessageTypeRequired
	}
	if sagaID == "" {
		return Message{}, ErrMessageSagaRequired
	}
	if orderID == "" {
		return Message{}, ErrMessageOrderRequired
	}

	encoded, err := encodePayload(payload)
	if err != nil {
		return Message{}, err
	}

	return Message{
		ID:         uuid.NewString(),
		Type:       messageType,
		SagaID:     sagaID,
		OrderID:    orderID,
		OccurredAt: time.Now().UTC(),
		Payload:    encoded,
	}, nil
}

func (m Message) DecodePayload(target any) error {
	if len(m.Payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(m.Payload, target); err != nil {
		return ErrMessagePayloadInvalid
	}
	return nil
}

func encodePayload(payload any) (json.RawMessage, error) {
	if payload == nil {
		return json.RawMessage("{}"), nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrMessagePayloadInvalid
	}
	return encoded, nil
}
