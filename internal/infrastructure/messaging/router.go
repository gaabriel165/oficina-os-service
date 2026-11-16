package messaging

import (
	"context"
	"fmt"

	domain "github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
)

type Router struct {
	handlers map[string]domain.Handler
}

func NewRouter() *Router {
	return &Router{handlers: map[string]domain.Handler{}}
}

func (r *Router) Register(messageType string, handler domain.Handler) {
	r.handlers[messageType] = handler
}

func (r *Router) Handle(ctx context.Context, message domain.Message) error {
	handler, found := r.handlers[message.Type]
	if !found {
		return fmt.Errorf("%w: %s", domain.ErrUnknownMessageType, message.Type)
	}
	return handler(ctx, message)
}
