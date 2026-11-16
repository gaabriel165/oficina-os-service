package repository

import (
	"context"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	domainrepo "github.com/gabrielcamargo/oficina-api/internal/domain/repository"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/database/model"
	"gorm.io/gorm"
)

type GormSagaRepository struct {
	db *gorm.DB
}

func NewGormSagaRepository(db *gorm.DB) *GormSagaRepository {
	return &GormSagaRepository{db: db}
}

func (r *GormSagaRepository) Create(ctx context.Context, order *entity.ServiceOrder, saga *entity.Saga, outgoing ...messaging.OutgoingMessage) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := saveOrder(tx, order); err != nil {
			return err
		}
		if err := tx.Create(model.SagaModelFromDomain(saga)).Error; err != nil {
			return err
		}
		if err := appendSagaSteps(tx, saga); err != nil {
			return err
		}
		return appendToOutbox(ctx, tx, outgoing)
	})
}

func (r *GormSagaRepository) Save(ctx context.Context, order *entity.ServiceOrder, saga *entity.Saga, outgoing ...messaging.OutgoingMessage) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := updateSaga(tx, saga); err != nil {
			return err
		}
		if err := saveOrder(tx, order); err != nil {
			return err
		}
		if err := appendSagaSteps(tx, saga); err != nil {
			return err
		}
		return appendToOutbox(ctx, tx, outgoing)
	})
}

func (r *GormSagaRepository) FindByID(ctx context.Context, id string) (*entity.Saga, error) {
	return r.findOne(ctx, "id = ?", id)
}

func (r *GormSagaRepository) FindByOrderID(ctx context.Context, orderID string) (*entity.Saga, error) {
	return r.findOne(ctx, "service_order_id = ?", orderID)
}

func (r *GormSagaRepository) FindOverdue(ctx context.Context, now time.Time, limit int) ([]*entity.Saga, error) {
	var models []model.SagaModel
	err := r.db.WithContext(ctx).
		Where("status = ? AND deadline IS NOT NULL AND deadline < ?", valueobject.SagaStatusRunning.String(), now).
		Order("deadline").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}

	sagas := make([]*entity.Saga, len(models))
	for i := range models {
		sagas[i] = models[i].ToDomain()
	}
	return sagas, nil
}

func (r *GormSagaRepository) findOne(ctx context.Context, condition, value string) (*entity.Saga, error) {
	var m model.SagaModel
	err := r.db.WithContext(ctx).
		Preload("Steps", func(db *gorm.DB) *gorm.DB { return db.Order("occurred_at") }).
		First(&m, condition, value).Error
	if isNotFound(err) {
		return nil, domainrepo.ErrSagaNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.ToDomain(), nil
}

func updateSaga(tx *gorm.DB, saga *entity.Saga) error {
	m := model.SagaModelFromDomain(saga)
	result := tx.Model(&model.SagaModel{}).
		Where("id = ? AND version = ?", m.ID, m.Version).
		Updates(map[string]any{
			"status":                m.Status,
			"step":                  m.Step,
			"pending_compensations": m.PendingCompensations,
			"failure_reason":        m.FailureReason,
			"deadline":              m.Deadline,
			"finished_at":           m.FinishedAt,
			"updated_at":            m.UpdatedAt,
			"version":               m.Version + 1,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrSagaConflict
	}
	return nil
}

func appendSagaSteps(tx *gorm.DB, saga *entity.Saga) error {
	steps := model.SagaStepModelsFromDomain(saga)
	if len(steps) == 0 {
		return nil
	}
	return tx.Create(&steps).Error
}
