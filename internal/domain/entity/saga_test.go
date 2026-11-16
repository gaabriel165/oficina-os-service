package entity_test

import (
	"testing"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSaga(t *testing.T) *entity.Saga {
	t.Helper()
	saga, err := entity.NewSaga("order-1", nil)
	require.NoError(t, err)
	return saga
}

func logNames(saga *entity.Saga) []string {
	names := []string{}
	for _, entry := range saga.Log() {
		names = append(names, entry.Kind.String()+":"+entry.Name)
	}
	return names
}

func TestNewSaga_ShouldStartRunningAtDiagnosis(t *testing.T) {
	deadline := time.Now().Add(time.Hour)

	saga, err := entity.NewSaga("order-1", &deadline)

	require.NoError(t, err)
	assert.NotEmpty(t, saga.ID())
	assert.Equal(t, "order-1", saga.OrderID())
	assert.Equal(t, valueobject.SagaStatusRunning, saga.Status())
	assert.Equal(t, valueobject.SagaStepDiagnosis, saga.Step())
	assert.True(t, saga.IsRunning())
	assert.True(t, saga.IsAt(valueobject.SagaStepDiagnosis))
	assert.False(t, saga.IsCompensating())
	assert.Equal(t, &deadline, saga.Deadline())
	assert.Zero(t, saga.Version())
	assert.Nil(t, saga.FinishedAt())
	assert.False(t, saga.CreatedAt().IsZero())
	assert.False(t, saga.UpdatedAt().IsZero())
	assert.Equal(t, []string{"outcome:SagaStarted"}, logNames(saga))
}

func TestNewSaga_ShouldRequireServiceOrder(t *testing.T) {
	_, err := entity.NewSaga("", nil)

	assert.ErrorIs(t, err, entity.ErrSagaOrderRequired)
}

func TestSagaRecord_ShouldAppendCommandsAndRepliesToLog(t *testing.T) {
	saga := newSaga(t)

	saga.RecordCommand("RequestDiagnosis")
	saga.RecordReply("DiagnosisFailed", "vehicle not delivered")

	assert.Equal(t, []string{"outcome:SagaStarted", "command:RequestDiagnosis", "reply:DiagnosisFailed"}, logNames(saga))
	assert.Equal(t, "vehicle not delivered", saga.Log()[2].Detail)
	assert.Len(t, saga.PendingLog(), 3)
}

func TestSagaMoveTo_ShouldChangeStepAndDeadline(t *testing.T) {
	saga := newSaga(t)
	deadline := time.Now().Add(time.Hour)

	err := saga.MoveTo(valueobject.SagaStepPayment, &deadline)

	require.NoError(t, err)
	assert.True(t, saga.IsAt(valueobject.SagaStepPayment))
	assert.Equal(t, &deadline, saga.Deadline())
}

func TestSagaMoveTo_ShouldFailWhenSagaIsFinished(t *testing.T) {
	saga := newSaga(t)
	require.NoError(t, saga.Complete())

	err := saga.MoveTo(valueobject.SagaStepPayment, nil)

	assert.ErrorIs(t, err, entity.ErrSagaNotRunning)
}

func TestSagaComplete_ShouldFinishSaga(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	saga, err := entity.NewSaga("order-1", &deadline)
	require.NoError(t, err)

	err = saga.Complete()

	require.NoError(t, err)
	assert.Equal(t, valueobject.SagaStatusCompleted, saga.Status())
	assert.NotNil(t, saga.FinishedAt())
	assert.Nil(t, saga.Deadline())
	assert.Contains(t, logNames(saga), "outcome:SagaCompleted")
}

func TestSagaComplete_ShouldFailWhenAlreadyCompleted(t *testing.T) {
	saga := newSaga(t)
	require.NoError(t, saga.Complete())

	err := saga.Complete()

	assert.ErrorIs(t, err, entity.ErrSagaNotRunning)
}

func TestSagaFail_ShouldStartCompensatingWithFirstCommand(t *testing.T) {
	saga := newSaga(t)

	next, err := saga.Fail("payment failed", []string{"CancelBudget", "CancelWorkOrder"})

	require.NoError(t, err)
	assert.Equal(t, "CancelBudget", next)
	assert.True(t, saga.IsCompensating())
	assert.False(t, saga.IsAt(valueobject.SagaStepDiagnosis))
	assert.Equal(t, "payment failed", saga.FailureReason())
	assert.Equal(t, []string{"CancelBudget", "CancelWorkOrder"}, saga.PendingCompensations())
	assert.Nil(t, saga.FinishedAt())
	assert.Equal(t, []string{"outcome:SagaStarted", "failure:diagnosis", "compensation:CancelBudget"}, logNames(saga))
}

func TestSagaFail_ShouldFinishImmediatelyWithoutCompensations(t *testing.T) {
	saga := newSaga(t)

	next, err := saga.Fail("diagnosis failed", nil)

	require.NoError(t, err)
	assert.Empty(t, next)
	assert.Equal(t, valueobject.SagaStatusCompensated, saga.Status())
	assert.NotNil(t, saga.FinishedAt())
	assert.Contains(t, logNames(saga), "outcome:SagaCompensated")
}

func TestSagaFail_ShouldRequireReason(t *testing.T) {
	saga := newSaga(t)

	_, err := saga.Fail("", []string{"CancelWorkOrder"})

	assert.ErrorIs(t, err, entity.ErrSagaReasonRequired)
	assert.True(t, saga.IsRunning())
}

func TestSagaFail_ShouldFailWhenNotRunning(t *testing.T) {
	saga := newSaga(t)
	require.NoError(t, saga.Complete())

	_, err := saga.Fail("too late", []string{"CancelWorkOrder"})

	assert.ErrorIs(t, err, entity.ErrSagaNotRunning)
}

func TestSagaConfirmCompensation_ShouldDispatchNextCommandInOrder(t *testing.T) {
	saga := newSaga(t)
	_, err := saga.Fail("out of stock", []string{"RefundPayment", "CancelBudget", "CancelWorkOrder"})
	require.NoError(t, err)

	next, confirmed := saga.ConfirmCompensation("RefundPayment")

	assert.True(t, confirmed)
	assert.Equal(t, "CancelBudget", next)
	assert.Equal(t, []string{"CancelBudget", "CancelWorkOrder"}, saga.PendingCompensations())
	assert.True(t, saga.IsCompensating())
}

func TestSagaConfirmCompensation_ShouldFinishAfterLastCommand(t *testing.T) {
	saga := newSaga(t)
	_, err := saga.Fail("budget rejected", []string{"CancelWorkOrder"})
	require.NoError(t, err)

	next, confirmed := saga.ConfirmCompensation("CancelWorkOrder")

	assert.True(t, confirmed)
	assert.Empty(t, next)
	assert.Equal(t, valueobject.SagaStatusCompensated, saga.Status())
	assert.Empty(t, saga.PendingCompensations())
	assert.NotNil(t, saga.FinishedAt())
}

func TestSagaConfirmCompensation_ShouldIgnoreOutOfOrderConfirmation(t *testing.T) {
	saga := newSaga(t)
	_, err := saga.Fail("payment failed", []string{"CancelBudget", "CancelWorkOrder"})
	require.NoError(t, err)

	next, confirmed := saga.ConfirmCompensation("CancelWorkOrder")

	assert.False(t, confirmed)
	assert.Empty(t, next)
	assert.Len(t, saga.PendingCompensations(), 2)
}

func TestSagaConfirmCompensation_ShouldIgnoreWhenNotCompensating(t *testing.T) {
	saga := newSaga(t)

	_, confirmed := saga.ConfirmCompensation("CancelWorkOrder")

	assert.False(t, confirmed)
}

func TestRestoreSaga_ShouldKeepSnapshotAndSeparateNewLogEntries(t *testing.T) {
	saga := entity.RestoreSaga(entity.SagaSnapshot{
		ID:      "saga-1",
		OrderID: "order-1",
		Status:  valueobject.SagaStatusRunning,
		Step:    valueobject.SagaStepPayment,
		Version: 3,
		Log:     []entity.SagaLogEntry{{ID: "log-1", Kind: valueobject.SagaLogKindOutcome, Name: "SagaStarted"}},
	})

	saga.RecordReply("PaymentConfirmed", "")

	assert.Equal(t, "saga-1", saga.ID())
	assert.Equal(t, 3, saga.Version())
	assert.True(t, saga.IsAt(valueobject.SagaStepPayment))
	assert.Len(t, saga.Log(), 2)
	assert.Len(t, saga.PendingLog(), 1)
}

func TestSagaStep_ShouldKnowItsOrder(t *testing.T) {
	assert.True(t, valueobject.SagaStepDiagnosis.IsBefore(valueobject.SagaStepPayment))
	assert.False(t, valueobject.SagaStepExecution.IsBefore(valueobject.SagaStepPayment))
	assert.Equal(t, "budget_approval", valueobject.SagaStepBudgetApproval.String())
	assert.Equal(t, "compensating", valueobject.SagaStatusCompensating.String())
}
