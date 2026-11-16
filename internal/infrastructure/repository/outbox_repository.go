package repository

import (
	"context"
	"encoding/json"
	"time"

	domain "github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/database/model"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/messaging"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormOutboxRepository struct {
	db *gorm.DB
}

func NewGormOutboxRepository(db *gorm.DB) *GormOutboxRepository {
	return &GormOutboxRepository{db: db}
}

func (r *GormOutboxRepository) Enqueue(ctx context.Context, outgoing ...domain.OutgoingMessage) error {
	return appendToOutbox(ctx, r.db.WithContext(ctx), outgoing)
}

func (r *GormOutboxRepository) RelayPending(ctx context.Context, limit int, relay domain.RelayFunc) (int, error) {
	relayed := 0

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pending []model.OutboxMessageModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("published_at IS NULL").
			Order("created_at").
			Limit(limit).
			Find(&pending).Error
		if err != nil {
			return err
		}

		for _, record := range pending {
			if err := relay(traceContextOf(ctx, record), outgoingFromModel(record)); err != nil {
				return err
			}
			if err := markPublished(tx, record.ID); err != nil {
				return err
			}
			relayed++
		}
		return nil
	})

	return relayed, err
}

func appendToOutbox(ctx context.Context, tx *gorm.DB, outgoing []domain.OutgoingMessage) error {
	if len(outgoing) == 0 {
		return nil
	}

	traceContext, err := json.Marshal(messaging.InjectTraceContext(ctx))
	if err != nil {
		return err
	}

	records := make([]model.OutboxMessageModel, len(outgoing))
	for index, item := range outgoing {
		records[index] = model.OutboxMessageModel{
			ID:           item.Message.ID,
			Destination:  string(item.Destination),
			MessageType:  item.Message.Type,
			SagaID:       item.Message.SagaID,
			OrderID:      item.Message.OrderID,
			Payload:      string(item.Message.Payload),
			TraceContext: string(traceContext),
			OccurredAt:   item.Message.OccurredAt,
		}
	}
	return tx.Create(&records).Error
}

func markPublished(tx *gorm.DB, id string) error {
	now := time.Now()
	return tx.Model(&model.OutboxMessageModel{}).
		Where("id = ?", id).
		Updates(map[string]any{"published_at": now, "updated_at": now}).Error
}

func outgoingFromModel(record model.OutboxMessageModel) domain.OutgoingMessage {
	return domain.OutgoingMessage{
		Destination: domain.Destination(record.Destination),
		Message: domain.Message{
			ID:         record.ID,
			Type:       record.MessageType,
			SagaID:     record.SagaID,
			OrderID:    record.OrderID,
			OccurredAt: record.OccurredAt,
			Payload:    json.RawMessage(record.Payload),
		},
	}
}

func traceContextOf(ctx context.Context, record model.OutboxMessageModel) context.Context {
	carrier := map[string]string{}
	if err := json.Unmarshal([]byte(record.TraceContext), &carrier); err != nil {
		return ctx
	}
	return messaging.ExtractTraceContext(ctx, carrier)
}
