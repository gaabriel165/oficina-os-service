package serviceorder

import (
	"context"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
)

type SagaCanceller interface {
	Cancel(ctx context.Context, orderID, reason string) (*entity.ServiceOrder, error)
}

type CancelServiceOrderInput struct {
	OrderID string
	Reason  string
}

type CancelServiceOrderUseCase struct {
	saga SagaCanceller
}

func NewCancelServiceOrderUseCase(saga SagaCanceller) *CancelServiceOrderUseCase {
	return &CancelServiceOrderUseCase{saga: saga}
}

func (u *CancelServiceOrderUseCase) Execute(ctx context.Context, input CancelServiceOrderInput) (*entity.ServiceOrder, error) {
	if input.Reason == "" {
		return nil, entity.ErrServiceOrderCancellationReasonRequired
	}
	return u.saga.Cancel(ctx, input.OrderID, input.Reason)
}
