package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
)

const (
	defaultBatchSize = 20
	defaultInterval  = time.Second
)

type Relay struct {
	outbox    messaging.OutboxRepository
	publisher messaging.PublisherService
	interval  time.Duration
	batchSize int
}

func NewRelay(outbox messaging.OutboxRepository, publisher messaging.PublisherService) *Relay {
	return &Relay{outbox: outbox, publisher: publisher, interval: defaultInterval, batchSize: defaultBatchSize}
}

func (r *Relay) WithInterval(interval time.Duration) *Relay {
	r.interval = interval
	return r
}

func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.drain(ctx)
		}
	}
}

func (r *Relay) RelayOnce(ctx context.Context) (int, error) {
	return r.outbox.RelayPending(ctx, r.batchSize, func(ctx context.Context, outgoing messaging.OutgoingMessage) error {
		return r.publisher.Publish(ctx, outgoing.Destination, outgoing.Message)
	})
}

func (r *Relay) drain(ctx context.Context) {
	for ctx.Err() == nil {
		relayed, err := r.RelayOnce(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "outbox.relay_failed",
				slog.String("event", "outbox.relay_failed"),
				slog.String("error", err.Error()),
			)
			return
		}
		if relayed < r.batchSize {
			return
		}
	}
}
