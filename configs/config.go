package configs

import (
	"os"
	"time"

	"github.com/joho/godotenv"
)

const defaultSagaStepTimeout = 72 * time.Hour

type Config struct {
	ServerPort             string
	Environment            string
	ServiceName            string
	DatabaseURL            string
	JWTSecret              string
	AWSRegion              string
	SQSEndpoint            string
	SagaRepliesQueue       string
	BillingCommandsQueue   string
	ExecutionCommandsQueue string
	SagaStepTimeout        time.Duration
	ResendAPIKey           string
	EmailFrom              string
	OtelExporterEndpoint   string
	OtelExporterHeaders    string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	return &Config{
		ServerPort:             getEnv("SERVER_PORT", "8080"),
		Environment:            getEnv("APP_ENV", "local"),
		ServiceName:            getEnv("OTEL_SERVICE_NAME", "oficina-api"),
		DatabaseURL:            getEnv("DATABASE_URL", ""),
		JWTSecret:              getEnv("JWT_SECRET", ""),
		AWSRegion:              getEnv("AWS_REGION", "us-east-1"),
		SQSEndpoint:            getEnv("SQS_ENDPOINT", ""),
		SagaRepliesQueue:       getEnv("SAGA_REPLIES_QUEUE", "oficina-saga-replies"),
		BillingCommandsQueue:   getEnv("BILLING_COMMANDS_QUEUE", "oficina-billing-commands"),
		ExecutionCommandsQueue: getEnv("EXECUTION_COMMANDS_QUEUE", "oficina-execution-commands"),
		SagaStepTimeout:        getDuration("SAGA_STEP_TIMEOUT", defaultSagaStepTimeout),
		ResendAPIKey:           getEnv("RESEND_API_KEY", ""),
		EmailFrom:              getEnv("EMAIL_FROM", "onboarding@resend.dev"),
		OtelExporterEndpoint:   getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OtelExporterHeaders:    getEnv("OTEL_EXPORTER_OTLP_HEADERS", ""),
	}, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}
