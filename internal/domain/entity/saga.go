package entity

import (
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
	"github.com/google/uuid"
)

const (
	sagaOutcomeStarted     = "SagaStarted"
	sagaOutcomeCompleted   = "SagaCompleted"
	sagaOutcomeCompensated = "SagaCompensated"
)

type SagaLogEntry struct {
	ID         string
	Kind       valueobject.SagaLogKind
	Name       string
	Detail     string
	OccurredAt time.Time
}

type Saga struct {
	id                   string
	orderID              string
	status               valueobject.SagaStatus
	step                 valueobject.SagaStep
	pendingCompensations []string
	failureReason        string
	deadline             *time.Time
	version              int
	finishedAt           *time.Time
	createdAt            time.Time
	updatedAt            time.Time
	log                  []SagaLogEntry
	pendingLog           []SagaLogEntry
}

type SagaSnapshot struct {
	ID                   string
	OrderID              string
	Status               valueobject.SagaStatus
	Step                 valueobject.SagaStep
	PendingCompensations []string
	FailureReason        string
	Deadline             *time.Time
	Version              int
	FinishedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Log                  []SagaLogEntry
}

func NewSaga(orderID string, deadline *time.Time) (*Saga, error) {
	if orderID == "" {
		return nil, ErrSagaOrderRequired
	}

	now := time.Now()
	saga := &Saga{
		id:        uuid.NewString(),
		orderID:   orderID,
		status:    valueobject.SagaStatusRunning,
		step:      valueobject.SagaStepDiagnosis,
		deadline:  deadline,
		createdAt: now,
		updatedAt: now,
	}
	saga.record(valueobject.SagaLogKindOutcome, sagaOutcomeStarted, "")
	return saga, nil
}

func RestoreSaga(snapshot SagaSnapshot) *Saga {
	return &Saga{
		id:                   snapshot.ID,
		orderID:              snapshot.OrderID,
		status:               snapshot.Status,
		step:                 snapshot.Step,
		pendingCompensations: snapshot.PendingCompensations,
		failureReason:        snapshot.FailureReason,
		deadline:             snapshot.Deadline,
		version:              snapshot.Version,
		finishedAt:           snapshot.FinishedAt,
		createdAt:            snapshot.CreatedAt,
		updatedAt:            snapshot.UpdatedAt,
		log:                  snapshot.Log,
	}
}

func (s *Saga) ID() string                     { return s.id }
func (s *Saga) OrderID() string                { return s.orderID }
func (s *Saga) Status() valueobject.SagaStatus { return s.status }
func (s *Saga) Step() valueobject.SagaStep     { return s.step }
func (s *Saga) PendingCompensations() []string { return s.pendingCompensations }
func (s *Saga) FailureReason() string          { return s.failureReason }
func (s *Saga) Deadline() *time.Time           { return s.deadline }
func (s *Saga) Version() int                   { return s.version }
func (s *Saga) FinishedAt() *time.Time         { return s.finishedAt }
func (s *Saga) CreatedAt() time.Time           { return s.createdAt }
func (s *Saga) UpdatedAt() time.Time           { return s.updatedAt }
func (s *Saga) PendingLog() []SagaLogEntry     { return s.pendingLog }

func (s *Saga) Log() []SagaLogEntry {
	return append(append([]SagaLogEntry{}, s.log...), s.pendingLog...)
}

func (s *Saga) IsRunning() bool {
	return s.status == valueobject.SagaStatusRunning
}

func (s *Saga) IsCompensating() bool {
	return s.status == valueobject.SagaStatusCompensating
}

func (s *Saga) IsAt(step valueobject.SagaStep) bool {
	return s.IsRunning() && s.step == step
}

func (s *Saga) RecordCommand(name string) {
	s.record(valueobject.SagaLogKindCommand, name, "")
}

func (s *Saga) RecordReply(name, detail string) {
	s.record(valueobject.SagaLogKindReply, name, detail)
}

func (s *Saga) MoveTo(step valueobject.SagaStep, deadline *time.Time) error {
	if !s.IsRunning() {
		return ErrSagaNotRunning
	}
	s.step = step
	s.deadline = deadline
	return nil
}

func (s *Saga) Complete() error {
	if !s.IsRunning() {
		return ErrSagaNotRunning
	}
	s.status = valueobject.SagaStatusCompleted
	s.finish()
	s.record(valueobject.SagaLogKindOutcome, sagaOutcomeCompleted, "")
	return nil
}

func (s *Saga) Fail(reason string, compensations []string) (string, error) {
	if reason == "" {
		return "", ErrSagaReasonRequired
	}
	if !s.IsRunning() {
		return "", ErrSagaNotRunning
	}

	s.failureReason = reason
	s.deadline = nil
	s.record(valueobject.SagaLogKindFailure, s.step.String(), reason)

	if len(compensations) == 0 {
		s.markCompensated()
		return "", nil
	}

	s.status = valueobject.SagaStatusCompensating
	s.pendingCompensations = compensations
	return s.dispatchNextCompensation(), nil
}

func (s *Saga) ConfirmCompensation(command string) (string, bool) {
	if !s.IsCompensating() || len(s.pendingCompensations) == 0 || s.pendingCompensations[0] != command {
		return "", false
	}

	s.pendingCompensations = s.pendingCompensations[1:]
	if len(s.pendingCompensations) == 0 {
		s.markCompensated()
		return "", true
	}
	return s.dispatchNextCompensation(), true
}

func (s *Saga) dispatchNextCompensation() string {
	next := s.pendingCompensations[0]
	s.record(valueobject.SagaLogKindCompensation, next, "")
	return next
}

func (s *Saga) markCompensated() {
	s.status = valueobject.SagaStatusCompensated
	s.pendingCompensations = nil
	s.finish()
	s.record(valueobject.SagaLogKindOutcome, sagaOutcomeCompensated, s.failureReason)
}

func (s *Saga) finish() {
	now := time.Now()
	s.finishedAt = &now
	s.deadline = nil
}

func (s *Saga) record(kind valueobject.SagaLogKind, name, detail string) {
	now := time.Now()
	s.updatedAt = now
	s.pendingLog = append(s.pendingLog, SagaLogEntry{
		ID:         uuid.NewString(),
		Kind:       kind,
		Name:       name,
		Detail:     detail,
		OccurredAt: now,
	})
}
