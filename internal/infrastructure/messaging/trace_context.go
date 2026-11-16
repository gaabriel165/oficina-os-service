package messaging

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	attributeMessageType = "message_type"
	attributeDataType    = "String"
)

func InjectTraceContext(ctx context.Context) map[string]string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier
}

func ExtractTraceContext(ctx context.Context, carrier map[string]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(carrier))
}

func toMessageAttributes(messageType string, carrier map[string]string) map[string]types.MessageAttributeValue {
	attributes := map[string]types.MessageAttributeValue{
		attributeMessageType: stringAttribute(messageType),
	}
	for key, value := range carrier {
		if value == "" {
			continue
		}
		attributes[key] = stringAttribute(value)
	}
	return attributes
}

func fromMessageAttributes(attributes map[string]types.MessageAttributeValue) map[string]string {
	carrier := map[string]string{}
	for key, value := range attributes {
		if value.StringValue == nil {
			continue
		}
		carrier[key] = *value.StringValue
	}
	return carrier
}

func stringAttribute(value string) types.MessageAttributeValue {
	return types.MessageAttributeValue{
		DataType:    aws.String(attributeDataType),
		StringValue: aws.String(value),
	}
}
