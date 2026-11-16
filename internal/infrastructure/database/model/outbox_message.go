package model

import "time"

type OutboxMessageModel struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	Destination  string
	MessageType  string
	SagaID       string `gorm:"type:uuid"`
	OrderID      string `gorm:"type:uuid"`
	Payload      string `gorm:"type:jsonb"`
	TraceContext string `gorm:"type:jsonb"`
	OccurredAt   time.Time
	PublishedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (OutboxMessageModel) TableName() string { return "outbox_messages" }
