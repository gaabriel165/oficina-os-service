package serviceorder

import (
	"context"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/repository"
)

type SagaStarter interface {
	Start(ctx context.Context, order *entity.ServiceOrder) (*entity.Saga, error)
}

type CreateServiceOrderInput struct {
	CustomerID string
	VehicleID  string
	Notes      string
}

type CreateServiceOrderUseCase struct {
	customerRepo repository.CustomerRepository
	vehicleRepo  repository.VehicleRepository
	saga         SagaStarter
}

func NewCreateServiceOrderUseCase(
	customerRepo repository.CustomerRepository,
	vehicleRepo repository.VehicleRepository,
	saga SagaStarter,
) *CreateServiceOrderUseCase {
	return &CreateServiceOrderUseCase{customerRepo: customerRepo, vehicleRepo: vehicleRepo, saga: saga}
}

func (u *CreateServiceOrderUseCase) Execute(ctx context.Context, input CreateServiceOrderInput) (*entity.ServiceOrder, error) {
	if _, err := u.customerRepo.FindByID(input.CustomerID); err != nil {
		return nil, err
	}

	vehicle, err := u.vehicleRepo.FindByID(input.VehicleID)
	if err != nil {
		return nil, err
	}

	if vehicle.CustomerID() != input.CustomerID {
		return nil, entity.ErrServiceOrderVehicleNotOwnedByCustomer
	}

	order, err := entity.NewServiceOrder(input.CustomerID, input.VehicleID, input.Notes)
	if err != nil {
		return nil, err
	}

	if _, err := u.saga.Start(ctx, order); err != nil {
		return nil, err
	}

	return order, nil
}
