package messaging

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	domain "github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName            = "oficina/messaging"
	eventMessagePublished = "message.published"
	eventMessageFailed    = "message.failed"
)

type SQSPublisher struct {
	client   QueueClient
	resolver *queueResolver
	queues   map[domain.Destination]string
}

func NewSQSPublisher(client QueueClient, queues map[domain.Destination]string) *SQSPublisher {
	return &SQSPublisher{client: client, resolver: newQueueResolver(client), queues: queues}
}

func (p *SQSPublisher) Publish(ctx context.Context, destination domain.Destination, message domain.Message) error {
	queueName, found := p.queues[destination]
	if !found {
		return domain.ErrUnknownDestination
	}

	ctx, span := otel.Tracer(tracerName).Start(ctx, "publish "+message.Type,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(messageSpanAttributes(queueName, message)...),
	)
	defer span.End()

	if err := p.send(ctx, queueName, message); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		observability.Failure(ctx, eventMessageFailed, err, messageLogAttributes(queueName, message, "publish")...)
		return err
	}

	observability.Event(ctx, eventMessagePublished, messageLogAttributes(queueName, message, "publish")...)
	return nil
}

func (p *SQSPublisher) send(ctx context.Context, queueName string, message domain.Message) error {
	queueURL, err := p.resolver.resolve(ctx, queueName)
	if err != nil {
		return err
	}

	body, err := EncodeMessage(message)
	if err != nil {
		return err
	}

	_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(body),
		MessageAttributes: toMessageAttributes(message.Type, InjectTraceContext(ctx)),
	})
	return err
}

func messageSpanAttributes(queueName string, message domain.Message) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("messaging.system", "aws_sqs"),
		attribute.String("messaging.destination.name", queueName),
		attribute.String("messaging.message.id", message.ID),
		attribute.String("messaging.message.type", message.Type),
		attribute.String("saga.id", message.SagaID),
		attribute.String("order.id", message.OrderID),
	}
}

func messageLogAttributes(queueName string, message domain.Message, operation string) []any {
	return []any{
		slog.String("operation", operation),
		slog.String("queue", queueName),
		slog.String("message_id", message.ID),
		slog.String("message_type", message.Type),
		slog.String("saga_id", message.SagaID),
		slog.String("order_id", message.OrderID),
	}
}
