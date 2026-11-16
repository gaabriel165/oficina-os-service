package outbox_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/application/outbox"
	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBrokerDown = errors.New("broker down")

type fakeOutbox struct {
	mutex     sync.Mutex
	pending   []messaging.OutgoingMessage
	relayErr  error
	callCount int
}

func (f *fakeOutbox) RelayPending(ctx context.Context, limit int, relay messaging.RelayFunc) (int, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.callCount++
	if f.relayErr != nil {
		return 0, f.relayErr
	}

	relayed := 0
	for len(f.pending) > 0 && relayed < limit {
		if err := relay(ctx, f.pending[0]); err != nil {
			return relayed, err
		}
		f.pending = f.pending[1:]
		relayed++
	}
	return relayed, nil
}

func (f *fakeOutbox) Enqueue(_ context.Context, outgoing ...messaging.OutgoingMessage) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.pending = append(f.pending, outgoing...)
	return nil
}

func (f *fakeOutbox) calls() int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.callCount
}

type fakePublisher struct {
	mutex     sync.Mutex
	published []messaging.OutgoingMessage
	err       error
}

func (f *fakePublisher) Publish(_ context.Context, destination messaging.Destination, message messaging.Message) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, messaging.OutgoingMessage{Destination: destination, Message: message})
	return nil
}

func (f *fakePublisher) count() int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return len(f.published)
}

func pendingReplies(total int) []messaging.OutgoingMessage {
	pending := make([]messaging.OutgoingMessage, total)
	for index := range pending {
		pending[index] = messaging.OutgoingMessage{
			Destination: messaging.DestinationSagaReplies,
			Message:     messaging.Message{ID: "message", Type: messaging.TypeBudgetApproved},
		}
	}
	return pending
}

func TestRelayOnce_ShouldPublishPendingMessages(t *testing.T) {
	outboxRepo := &fakeOutbox{pending: pendingReplies(2)}
	publisher := &fakePublisher{}

	relayed, err := outbox.NewRelay(outboxRepo, publisher).RelayOnce(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, relayed)
	assert.Equal(t, 2, publisher.count())
	assert.Equal(t, messaging.DestinationSagaReplies, publisher.published[0].Destination)
}

func TestRelayOnce_ShouldReturnErrorWhenPublishFails(t *testing.T) {
	outboxRepo := &fakeOutbox{pending: pendingReplies(1)}
	publisher := &fakePublisher{err: errBrokerDown}

	relayed, err := outbox.NewRelay(outboxRepo, publisher).RelayOnce(context.Background())

	assert.ErrorIs(t, err, errBrokerDown)
	assert.Zero(t, relayed)
	assert.Len(t, outboxRepo.pending, 1)
}

func TestRun_ShouldDrainOutboxAcrossBatches(t *testing.T) {
	outboxRepo := &fakeOutbox{pending: pendingReplies(45)}
	publisher := &fakePublisher{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go outbox.NewRelay(outboxRepo, publisher).WithInterval(time.Millisecond).Run(ctx)

	assert.Eventually(t, func() bool { return publisher.count() == 45 }, time.Second, 5*time.Millisecond)
}

func TestRun_ShouldKeepRunningAfterRelayError(t *testing.T) {
	outboxRepo := &fakeOutbox{relayErr: errBrokerDown}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go outbox.NewRelay(outboxRepo, &fakePublisher{}).WithInterval(time.Millisecond).Run(ctx)

	assert.Eventually(t, func() bool { return outboxRepo.calls() >= 3 }, time.Second, 5*time.Millisecond)
}

func TestRun_ShouldStopWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})

	go func() {
		outbox.NewRelay(&fakeOutbox{}, &fakePublisher{}).WithInterval(time.Millisecond).Run(ctx)
		close(finished)
	}()
	cancel()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("relay did not stop after context cancellation")
	}
}
