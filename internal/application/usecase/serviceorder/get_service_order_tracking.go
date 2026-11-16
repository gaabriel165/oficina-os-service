package serviceorder

import (
	"context"

	"github.com/gabrielcamargo/oficina-api/internal/application/usecase"
	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/repository"
)

type TrackingRequester struct {
	ID         string
	IsCustomer bool
}

type GetServiceOrderHistoryUseCase struct {
	orderRepo repository.ServiceOrderRepository
}

func NewGetServiceOrderHistoryUseCase(orderRepo repository.ServiceOrderRepository) *GetServiceOrderHistoryUseCase {
	return &GetServiceOrderHistoryUseCase{orderRepo: orderRepo}
}

func (u *GetServiceOrderHistoryUseCase) Execute(ctx context.Context, orderID string, requester TrackingRequester) ([]entity.StatusChange, error) {
	if err := ensureAccess(ctx, u.orderRepo, orderID, requester); err != nil {
		return nil, err
	}
	return u.orderRepo.FindHistory(ctx, orderID)
}

type GetServiceOrderSagaUseCase struct {
	orderRepo repository.ServiceOrderRepository
	sagaRepo  repository.SagaRepository
}

func NewGetServiceOrderSagaUseCase(
	orderRepo repository.ServiceOrderRepository,
	sagaRepo repository.SagaRepository,
) *GetServiceOrderSagaUseCase {
	return &GetServiceOrderSagaUseCase{orderRepo: orderRepo, sagaRepo: sagaRepo}
}

func (u *GetServiceOrderSagaUseCase) Execute(ctx context.Context, orderID string, requester TrackingRequester) (*entity.Saga, error) {
	if err := ensureAccess(ctx, u.orderRepo, orderID, requester); err != nil {
		return nil, err
	}
	return u.sagaRepo.FindByOrderID(ctx, orderID)
}

func ensureAccess(ctx context.Context, orderRepo repository.ServiceOrderRepository, orderID string, requester TrackingRequester) error {
	order, err := orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return err
	}
	if requester.IsCustomer && !order.BelongsTo(requester.ID) {
		return usecase.ErrForbidden
	}
	return nil
}
