package repository

import (
	"context"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
)

type ServiceOrderRepository interface {
	Update(ctx context.Context, order *entity.ServiceOrder) error
	FindByID(ctx context.Context, id string) (*entity.ServiceOrder, error)
	FindByStatuses(ctx context.Context, statuses []valueobject.OrderStatus) ([]*entity.ServiceOrder, error)
	FindHistory(ctx context.Context, orderID string) ([]entity.StatusChange, error)
}
