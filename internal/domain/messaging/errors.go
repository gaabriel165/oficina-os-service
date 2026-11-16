package messaging

import "errors"

var (
	ErrMessageTypeRequired   = errors.New("message type is required")
	ErrMessageSagaRequired   = errors.New("message saga id is required")
	ErrMessageOrderRequired  = errors.New("message order id is required")
	ErrMessagePayloadInvalid = errors.New("message payload is invalid")
	ErrUnknownMessageType    = errors.New("unknown message type")
	ErrUnknownDestination    = errors.New("unknown message destination")
)
