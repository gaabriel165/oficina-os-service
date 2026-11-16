package model

import (
	"strings"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
)

const compensationSeparator = ","

type SagaStepModel struct {
	ID         string    `gorm:"type:uuid;primaryKey;column:id"`
	SagaID     string    `gorm:"type:uuid;not null;column:saga_id"`
	Kind       string    `gorm:"column:kind"`
	Name       string    `gorm:"column:name"`
	Detail     string    `gorm:"column:detail"`
	OccurredAt time.Time `gorm:"column:occurred_at"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (SagaStepModel) TableName() string { return "saga_steps" }

type SagaModel struct {
	ID                   string          `gorm:"type:uuid;primaryKey;column:id"`
	ServiceOrderID       string          `gorm:"type:uuid;not null;column:service_order_id"`
	Status               string          `gorm:"column:status"`
	Step                 string          `gorm:"column:step"`
	PendingCompensations string          `gorm:"column:pending_compensations"`
	FailureReason        string          `gorm:"column:failure_reason"`
	Deadline             *time.Time      `gorm:"column:deadline"`
	Version              int             `gorm:"column:version"`
	FinishedAt           *time.Time      `gorm:"column:finished_at"`
	CreatedAt            time.Time       `gorm:"column:created_at"`
	UpdatedAt            time.Time       `gorm:"column:updated_at"`
	Steps                []SagaStepModel `gorm:"foreignKey:SagaID"`
}

func (SagaModel) TableName() string { return "sagas" }

func SagaModelFromDomain(saga *entity.Saga) *SagaModel {
	return &SagaModel{
		ID:                   saga.ID(),
		ServiceOrderID:       saga.OrderID(),
		Status:               saga.Status().String(),
		Step:                 saga.Step().String(),
		PendingCompensations: strings.Join(saga.PendingCompensations(), compensationSeparator),
		FailureReason:        saga.FailureReason(),
		Deadline:             saga.Deadline(),
		Version:              saga.Version(),
		FinishedAt:           saga.FinishedAt(),
		CreatedAt:            saga.CreatedAt(),
		UpdatedAt:            saga.UpdatedAt(),
	}
}

func SagaStepModelsFromDomain(saga *entity.Saga) []SagaStepModel {
	models := make([]SagaStepModel, 0, len(saga.PendingLog()))
	for _, entry := range saga.PendingLog() {
		models = append(models, SagaStepModel{
			ID:         entry.ID,
			SagaID:     saga.ID(),
			Kind:       entry.Kind.String(),
			Name:       entry.Name,
			Detail:     entry.Detail,
			OccurredAt: entry.OccurredAt,
			CreatedAt:  entry.OccurredAt,
			UpdatedAt:  entry.OccurredAt,
		})
	}
	return models
}

func (m *SagaModel) ToDomain() *entity.Saga {
	log := make([]entity.SagaLogEntry, 0, len(m.Steps))
	for _, step := range m.Steps {
		log = append(log, entity.SagaLogEntry{
			ID:         step.ID,
			Kind:       valueobject.SagaLogKind(step.Kind),
			Name:       step.Name,
			Detail:     step.Detail,
			OccurredAt: step.OccurredAt,
		})
	}

	return entity.RestoreSaga(entity.SagaSnapshot{
		ID:                   m.ID,
		OrderID:              m.ServiceOrderID,
		Status:               valueobject.SagaStatus(m.Status),
		Step:                 valueobject.SagaStep(m.Step),
		PendingCompensations: splitCompensations(m.PendingCompensations),
		FailureReason:        m.FailureReason,
		Deadline:             m.Deadline,
		Version:              m.Version,
		FinishedAt:           m.FinishedAt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
		Log:                  log,
	})
}

func splitCompensations(joined string) []string {
	if joined == "" {
		return nil
	}
	return strings.Split(joined, compensationSeparator)
}
