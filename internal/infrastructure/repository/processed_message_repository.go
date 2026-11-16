package repository

import (
	"context"

	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormProcessedMessageRepository struct {
	db *gorm.DB
}

func NewGormProcessedMessageRepository(db *gorm.DB) *GormProcessedMessageRepository {
	return &GormProcessedMessageRepository{db: db}
}

func (r *GormProcessedMessageRepository) Exists(ctx context.Context, messageID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ProcessedMessageModel{}).Where("id = ?", messageID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormProcessedMessageRepository) Save(ctx context.Context, messageID string) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&model.ProcessedMessageModel{ID: messageID}).Error
}
