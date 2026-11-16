package messaging_test

import (
	"context"
	"testing"

	domain "github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/messaging"
	"github.com/stretchr/testify/assert"
)

func TestRouterHandle_ShouldDispatchToRegisteredHandler(t *testing.T) {
	handler := &recordingHandler{}
	router := messaging.NewRouter()
	router.Register(domain.TypeRequestPayment, handler.handle)

	err := router.Handle(context.Background(), domain.Message{ID: "message-1", Type: domain.TypeRequestPayment})

	assert.NoError(t, err)
	assert.Len(t, handler.received, 1)
}

func TestRouterHandle_ShouldReturnHandlerError(t *testing.T) {
	handler := &recordingHandler{err: errHandlerFailed}
	router := messaging.NewRouter()
	router.Register(domain.TypeRequestPayment, handler.handle)

	err := router.Handle(context.Background(), domain.Message{Type: domain.TypeRequestPayment})

	assert.ErrorIs(t, err, errHandlerFailed)
}

func TestRouterHandle_ShouldReturnErrorWhenTypeIsNotRegistered(t *testing.T) {
	err := messaging.NewRouter().Handle(context.Background(), domain.Message{Type: "Unknown"})

	assert.ErrorIs(t, err, domain.ErrUnknownMessageType)
}
