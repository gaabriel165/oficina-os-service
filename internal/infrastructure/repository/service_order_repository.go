package repository

import (
	"context"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	domainrepo "github.com/gabrielcamargo/oficina-api/internal/domain/repository"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/database/model"
	"gorm.io/gorm"
)

type GormServiceOrderRepository struct {
	db *gorm.DB
}

func NewGormServiceOrderRepository(db *gorm.DB) *GormServiceOrderRepository {
	return &GormServiceOrderRepository{db: db}
}

func (r *GormServiceOrderRepository) Update(ctx context.Context, order *entity.ServiceOrder) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return saveOrder(tx, order)
	})
}

func (r *GormServiceOrderRepository) FindByID(ctx context.Context, id string) (*entity.ServiceOrder, error) {
	var m model.ServiceOrderModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if isNotFound(err) {
		return nil, domainrepo.ErrServiceOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.ToDomain(), nil
}

func (r *GormServiceOrderRepository) FindByStatuses(ctx context.Context, statuses []valueobject.OrderStatus) ([]*entity.ServiceOrder, error) {
	values := make([]string, len(statuses))
	for i, status := range statuses {
		values[i] = status.String()
	}

	var models []model.ServiceOrderModel
	if err := r.db.WithContext(ctx).Where("status IN ?", values).Find(&models).Error; err != nil {
		return nil, err
	}

	orders := make([]*entity.ServiceOrder, len(models))
	for i, m := range models {
		orders[i] = m.ToDomain()
	}
	return orders, nil
}

func (r *GormServiceOrderRepository) FindHistory(ctx context.Context, orderID string) ([]entity.StatusChange, error) {
	var models []model.ServiceOrderStatusHistoryModel
	err := r.db.WithContext(ctx).Where("service_order_id = ?", orderID).Order("occurred_at").Find(&models).Error
	if isNotFound(err) {
		return []entity.StatusChange{}, nil
	}
	if err != nil {
		return nil, err
	}

	history := make([]entity.StatusChange, len(models))
	for i, m := range models {
		history[i] = m.ToDomain()
	}
	return history, nil
}

func saveOrder(tx *gorm.DB, order *entity.ServiceOrder) error {
	if err := tx.Save(model.ServiceOrderModelFromDomain(order)).Error; err != nil {
		return err
	}

	history := model.StatusHistoryModelsFromDomain(order)
	if len(history) == 0 {
		return nil
	}
	return tx.Create(&history).Error
}
