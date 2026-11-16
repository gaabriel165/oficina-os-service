package messaging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	domain "github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

const commandsQueue = "oficina-saga-replies"

var errHandlerFailed = errors.New("handler failed")

type recordingHandler struct {
	received []domain.Message
	contexts []context.Context
	err      error
}

func (h *recordingHandler) handle(ctx context.Context, message domain.Message) error {
	h.received = append(h.received, message)
	h.contexts = append(h.contexts, ctx)
	return h.err
}

func queuedMessage(t *testing.T, message domain.Message, attributes map[string]types.MessageAttributeValue) types.Message {
	t.Helper()
	body, err := messaging.EncodeMessage(message)
	require.NoError(t, err)
	return types.Message{
		MessageId:         aws.String("sqs-" + message.ID),
		ReceiptHandle:     aws.String("receipt-" + message.ID),
		Body:              aws.String(body),
		MessageAttributes: attributes,
	}
}

func newCommand(t *testing.T) domain.Message {
	t.Helper()
	message, err := domain.NewMessage(domain.TypeRequestPayment, "saga-1", "order-1", nil)
	require.NoError(t, err)
	return message
}

func TestPoll_ShouldHandleAndAcknowledgeMessage(t *testing.T) {
	command := newCommand(t)
	client := &fakeQueueClient{pending: []types.Message{queuedMessage(t, command, nil)}}
	handler := &recordingHandler{}
	processed := newFakeProcessedMessages()

	err := messaging.NewSQSConsumer(client, commandsQueue, handler.handle, processed).Poll(context.Background())

	require.NoError(t, err)
	require.Len(t, handler.received, 1)
	assert.Equal(t, command.ID, handler.received[0].ID)
	assert.Equal(t, []string{"receipt-" + command.ID}, client.deletedHandles)
	assert.True(t, processed.saved[command.ID])
}

func TestPoll_ShouldSkipHandlerWhenMessageWasAlreadyProcessed(t *testing.T) {
	command := newCommand(t)
	client := &fakeQueueClient{pending: []types.Message{queuedMessage(t, command, nil)}}
	handler := &recordingHandler{}
	processed := newFakeProcessedMessages()
	processed.saved[command.ID] = true

	err := messaging.NewSQSConsumer(client, commandsQueue, handler.handle, processed).Poll(context.Background())

	require.NoError(t, err)
	assert.Empty(t, handler.received)
	assert.Equal(t, []string{"receipt-" + command.ID}, client.deletedHandles)
}

func TestPoll_ShouldKeepMessageInQueueWhenHandlerFails(t *testing.T) {
	command := newCommand(t)
	client := &fakeQueueClient{pending: []types.Message{queuedMessage(t, command, nil)}}
	handler := &recordingHandler{err: errHandlerFailed}
	processed := newFakeProcessedMessages()

	err := messaging.NewSQSConsumer(client, commandsQueue, handler.handle, processed).Poll(context.Background())

	require.NoError(t, err)
	assert.Empty(t, client.deletedHandles)
	assert.False(t, processed.saved[command.ID])
}

func TestPoll_ShouldKeepMessageInQueueWhenBodyIsInvalid(t *testing.T) {
	client := &fakeQueueClient{pending: []types.Message{{
		MessageId:     aws.String("sqs-1"),
		ReceiptHandle: aws.String("receipt-1"),
		Body:          aws.String("not-json"),
	}}}
	handler := &recordingHandler{}

	err := messaging.NewSQSConsumer(client, commandsQueue, handler.handle, newFakeProcessedMessages()).Poll(context.Background())

	require.NoError(t, err)
	assert.Empty(t, handler.received)
	assert.Empty(t, client.deletedHandles)
}

func TestPoll_ShouldKeepMessageInQueueWhenIdempotencyCheckFails(t *testing.T) {
	command := newCommand(t)
	client := &fakeQueueClient{pending: []types.Message{queuedMessage(t, command, nil)}}
	handler := &recordingHandler{}
	processed := newFakeProcessedMessages()
	processed.existsErr = errQueueUnavailable

	err := messaging.NewSQSConsumer(client, commandsQueue, handler.handle, processed).Poll(context.Background())

	require.NoError(t, err)
	assert.Empty(t, handler.received)
	assert.Empty(t, client.deletedHandles)
}

func TestPoll_ShouldKeepMessageInQueueWhenProcessedMarkFails(t *testing.T) {
	command := newCommand(t)
	client := &fakeQueueClient{pending: []types.Message{queuedMessage(t, command, nil)}}
	handler := &recordingHandler{}
	processed := newFakeProcessedMessages()
	processed.saveErr = errQueueUnavailable

	err := messaging.NewSQSConsumer(client, commandsQueue, handler.handle, processed).Poll(context.Background())

	require.NoError(t, err)
	assert.Len(t, handler.received, 1)
	assert.Empty(t, client.deletedHandles)
}

func TestPoll_ShouldNotFailWhenAcknowledgeFails(t *testing.T) {
	command := newCommand(t)
	client := &fakeQueueClient{pending: []types.Message{queuedMessage(t, command, nil)}, deleteErr: errQueueUnavailable}
	handler := &recordingHandler{}

	err := messaging.NewSQSConsumer(client, commandsQueue, handler.handle, newFakeProcessedMessages()).Poll(context.Background())

	require.NoError(t, err)
	assert.Len(t, handler.received, 1)
}

func TestPoll_ShouldContinuePublisherTrace(t *testing.T) {
	enableTracing(t)
	publisherClient := &fakeQueueClient{}
	command := newCommand(t)
	publisher := messaging.NewSQSPublisher(publisherClient, map[domain.Destination]string{
		domain.DestinationBillingCommands: commandsQueue,
	})
	require.NoError(t, publisher.Publish(context.Background(), domain.DestinationBillingCommands, command))
	sent := publisherClient.sent[0]

	consumerClient := &fakeQueueClient{pending: []types.Message{{
		MessageId:         aws.String("sqs-1"),
		ReceiptHandle:     aws.String("receipt-1"),
		Body:              sent.MessageBody,
		MessageAttributes: sent.MessageAttributes,
	}}}
	handler := &recordingHandler{}

	err := messaging.NewSQSConsumer(consumerClient, commandsQueue, handler.handle, newFakeProcessedMessages()).Poll(context.Background())

	require.NoError(t, err)
	require.Len(t, handler.contexts, 1)
	traceparent := aws.ToString(sent.MessageAttributes["traceparent"].StringValue)
	consumerTraceID := trace.SpanContextFromContext(handler.contexts[0]).TraceID().String()
	assert.Contains(t, traceparent, consumerTraceID)
}

func TestPoll_ShouldReturnErrorWhenQueueCannotBeResolved(t *testing.T) {
	client := &fakeQueueClient{resolveErr: errQueueUnavailable}

	err := messaging.NewSQSConsumer(client, commandsQueue, (&recordingHandler{}).handle, newFakeProcessedMessages()).Poll(context.Background())

	assert.ErrorIs(t, err, errQueueUnavailable)
}

func TestPoll_ShouldReturnErrorWhenReceiveFails(t *testing.T) {
	client := &fakeQueueClient{receiveErr: errQueueUnavailable}

	err := messaging.NewSQSConsumer(client, commandsQueue, (&recordingHandler{}).handle, newFakeProcessedMessages()).Poll(context.Background())

	assert.ErrorIs(t, err, errQueueUnavailable)
}

func TestRun_ShouldStopWhenContextIsCancelled(t *testing.T) {
	client := &fakeQueueClient{receiveErr: errQueueUnavailable}
	consumer := messaging.NewSQSConsumer(client, commandsQueue, (&recordingHandler{}).handle, newFakeProcessedMessages()).
		WithPollBackoff(time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	finished := make(chan struct{})
	go func() {
		consumer.Run(ctx)
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("consumer did not stop after context cancellation")
	}
}
