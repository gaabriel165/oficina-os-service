package messaging_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	domain "github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const repliesQueue = "oficina-saga-replies"

func newPublisher(client messaging.QueueClient) *messaging.SQSPublisher {
	return messaging.NewSQSPublisher(client, map[domain.Destination]string{
		domain.DestinationSagaReplies: repliesQueue,
	})
}

func newReply(t *testing.T) domain.Message {
	t.Helper()
	message, err := domain.NewMessage(domain.TypeBudgetApproved, "saga-1", "order-1", domain.BudgetDecisionPayload{BudgetID: "budget-1"})
	require.NoError(t, err)
	return message
}

func enableTracing(t *testing.T) {
	t.Helper()
	provider := sdktrace.NewTracerProvider()
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
}

func TestPublish_ShouldSendEnvelopeToResolvedQueue(t *testing.T) {
	client := &fakeQueueClient{}
	message := newReply(t)

	err := newPublisher(client).Publish(context.Background(), domain.DestinationSagaReplies, message)

	require.NoError(t, err)
	require.Len(t, client.sent, 1)
	assert.Equal(t, "http://queues/"+repliesQueue, aws.ToString(client.sent[0].QueueUrl))

	decoded, err := messaging.DecodeMessage(aws.ToString(client.sent[0].MessageBody))
	require.NoError(t, err)
	assert.Equal(t, message.ID, decoded.ID)
	assert.Equal(t, message.Type, decoded.Type)
	assert.Equal(t, message.SagaID, decoded.SagaID)
	assert.Equal(t, message.OrderID, decoded.OrderID)
	assert.JSONEq(t, string(message.Payload), string(decoded.Payload))
}

func TestPublish_ShouldAttachMessageTypeAttribute(t *testing.T) {
	client := &fakeQueueClient{}

	err := newPublisher(client).Publish(context.Background(), domain.DestinationSagaReplies, newReply(t))

	require.NoError(t, err)
	attribute := client.sent[0].MessageAttributes["message_type"]
	assert.Equal(t, domain.TypeBudgetApproved, aws.ToString(attribute.StringValue))
}

func TestPublish_ShouldPropagateTraceContext(t *testing.T) {
	enableTracing(t)
	client := &fakeQueueClient{}

	err := newPublisher(client).Publish(context.Background(), domain.DestinationSagaReplies, newReply(t))

	require.NoError(t, err)
	attribute, found := client.sent[0].MessageAttributes["traceparent"]
	require.True(t, found)
	assert.NotEmpty(t, aws.ToString(attribute.StringValue))
}

func TestPublish_ShouldResolveQueueURLOnlyOnce(t *testing.T) {
	client := &fakeQueueClient{}
	publisher := newPublisher(client)

	require.NoError(t, publisher.Publish(context.Background(), domain.DestinationSagaReplies, newReply(t)))
	require.NoError(t, publisher.Publish(context.Background(), domain.DestinationSagaReplies, newReply(t)))

	assert.Equal(t, 1, client.resolveCalls)
}

func TestPublish_ShouldReturnErrorWhenDestinationIsUnknown(t *testing.T) {
	client := &fakeQueueClient{}

	err := newPublisher(client).Publish(context.Background(), domain.DestinationExecutionCommands, newReply(t))

	assert.ErrorIs(t, err, domain.ErrUnknownDestination)
	assert.Empty(t, client.sent)
}

func TestPublish_ShouldReturnErrorWhenQueueCannotBeResolved(t *testing.T) {
	client := &fakeQueueClient{resolveErr: errQueueUnavailable}

	err := newPublisher(client).Publish(context.Background(), domain.DestinationSagaReplies, newReply(t))

	assert.ErrorIs(t, err, errQueueUnavailable)
}

func TestPublish_ShouldReturnErrorWhenSendFails(t *testing.T) {
	client := &fakeQueueClient{sendErr: errQueueUnavailable}

	err := newPublisher(client).Publish(context.Background(), domain.DestinationSagaReplies, newReply(t))

	assert.ErrorIs(t, err, errQueueUnavailable)
}
