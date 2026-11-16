package repository

import (
	"context"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
)

type SagaRepository interface {
	Create(ctx context.Context, order *entity.ServiceOrder, saga *entity.Saga, outgoing ...messaging.OutgoingMessage) error
	Save(ctx context.Context, order *entity.ServiceOrder, saga *entity.Saga, outgoing ...messaging.OutgoingMessage) error
	FindByID(ctx context.Context, id string) (*entity.Saga, error)
	FindByOrderID(ctx context.Context, orderID string) (*entity.Saga, error)
	FindOverdue(ctx context.Context, now time.Time, limit int) ([]*entity.Saga, error)
}
