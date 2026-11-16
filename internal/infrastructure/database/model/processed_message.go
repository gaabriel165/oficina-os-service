package model

import "time"

type ProcessedMessageModel struct {
	ID        string `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (ProcessedMessageModel) TableName() string { return "processed_messages" }
