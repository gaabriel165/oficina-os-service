package messaging

import "context"

type Destination string

const (
	DestinationExecutionCommands Destination = "execution-commands"
	DestinationBillingCommands   Destination = "billing-commands"
	DestinationSagaReplies       Destination = "saga-replies"
)

type OutgoingMessage struct {
	Destination Destination
	Message     Message
}

type Handler func(ctx context.Context, message Message) error

type PublisherService interface {
	Publish(ctx context.Context, destination Destination, message Message) error
}

type ProcessedMessageRepository interface {
	Exists(ctx context.Context, messageID string) (bool, error)
	Save(ctx context.Context, messageID string) error
}

type RelayFunc func(ctx context.Context, outgoing OutgoingMessage) error

type OutboxRepository interface {
	Enqueue(ctx context.Context, outgoing ...OutgoingMessage) error
	RelayPending(ctx context.Context, limit int, relay RelayFunc) (int, error)
}
