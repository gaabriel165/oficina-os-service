package messaging

import (
	"context"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	domain "github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	eventMessageProcessed = "message.processed"
	eventMessageDuplicate = "message.duplicate"
	eventMessageInvalid   = "message.invalid"
	eventQueuePollFailed  = "queue.poll_failed"

	defaultWaitTimeSeconds   = 10
	defaultMaxMessages       = 10
	defaultVisibilityTimeout = 30
	defaultPollBackoff       = 2 * time.Second
)

type SQSConsumer struct {
	client      QueueClient
	resolver    *queueResolver
	queueName   string
	handler     domain.Handler
	processed   domain.ProcessedMessageRepository
	pollBackoff time.Duration
}

func NewSQSConsumer(
	client QueueClient,
	queueName string,
	handler domain.Handler,
	processed domain.ProcessedMessageRepository,
) *SQSConsumer {
	return &SQSConsumer{
		client:      client,
		resolver:    newQueueResolver(client),
		queueName:   queueName,
		handler:     handler,
		processed:   processed,
		pollBackoff: defaultPollBackoff,
	}
}

func (c *SQSConsumer) WithPollBackoff(backoff time.Duration) *SQSConsumer {
	c.pollBackoff = backoff
	return c
}

func (c *SQSConsumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := c.Poll(ctx); err != nil && ctx.Err() == nil {
			observability.Failure(ctx, eventQueuePollFailed, err, slog.String("queue", c.queueName))
			c.wait(ctx)
		}
	}
}

func (c *SQSConsumer) Poll(ctx context.Context) error {
	queueURL, err := c.resolver.resolve(ctx, c.queueName)
	if err != nil {
		return err
	}

	output, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(queueURL),
		MaxNumberOfMessages:   defaultMaxMessages,
		WaitTimeSeconds:       defaultWaitTimeSeconds,
		VisibilityTimeout:     defaultVisibilityTimeout,
		MessageAttributeNames: []string{"All"},
	})
	if err != nil {
		return err
	}

	for _, received := range output.Messages {
		c.process(ctx, queueURL, received)
	}
	return nil
}

func (c *SQSConsumer) process(ctx context.Context, queueURL string, received types.Message) {
	message, err := DecodeMessage(aws.ToString(received.Body))
	if err != nil {
		observability.Failure(ctx, eventMessageInvalid, err,
			slog.String("queue", c.queueName),
			slog.String("sqs_message_id", aws.ToString(received.MessageId)),
		)
		return
	}

	ctx = ExtractTraceContext(ctx, fromMessageAttributes(received.MessageAttributes))
	ctx, span := otel.Tracer(tracerName).Start(ctx, "process "+message.Type,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(messageSpanAttributes(c.queueName, message)...),
	)
	defer span.End()

	outcome, err := c.handle(ctx, message)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		observability.Failure(ctx, eventMessageFailed, err, messageLogAttributes(c.queueName, message, "process")...)
		return
	}

	if err := c.acknowledge(ctx, queueURL, received); err != nil {
		span.RecordError(err)
		observability.Failure(ctx, eventMessageFailed, err, messageLogAttributes(c.queueName, message, "acknowledge")...)
		return
	}

	observability.Event(ctx, outcome, messageLogAttributes(c.queueName, message, "process")...)
}

func (c *SQSConsumer) handle(ctx context.Context, message domain.Message) (string, error) {
	alreadyProcessed, err := c.processed.Exists(ctx, message.ID)
	if err != nil {
		return "", err
	}
	if alreadyProcessed {
		return eventMessageDuplicate, nil
	}

	if err := c.handler(ctx, message); err != nil {
		return "", err
	}

	if err := c.processed.Save(ctx, message.ID); err != nil {
		return "", err
	}
	return eventMessageProcessed, nil
}

func (c *SQSConsumer) acknowledge(ctx context.Context, queueURL string, received types.Message) error {
	_, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: received.ReceiptHandle,
	})
	return err
}

func (c *SQSConsumer) wait(ctx context.Context) {
	timer := time.NewTimer(c.pollBackoff)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
