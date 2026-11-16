package saga_test

import (
	"testing"
	"time"

	"github.com/gabrielcamargo/oficina-api/internal/application/mocks"
	"github.com/gabrielcamargo/oficina-api/internal/application/saga"
	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/messaging"
	"github.com/gabrielcamargo/oficina-api/internal/domain/repository"
	"github.com/gabrielcamargo/oficina-api/internal/domain/valueobject"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStart_ShouldCreateSagaAndRequestDiagnosis(t *testing.T) {
	s := newScenario(t)

	created, err := s.orchestrator.Start(ctx, s.order)

	require.NoError(t, err)
	assert.True(t, created.IsAt(valueobject.SagaStepDiagnosis))
	assert.NotNil(t, created.Deadline())
	requested := s.store.lastPublished(t)
	assert.Equal(t, messaging.DestinationExecutionCommands, requested.Destination)
	assert.Equal(t, messaging.TypeRequestDiagnosis, requested.Message.Type)
	assert.Equal(t, s.order.ID(), requested.Message.OrderID)
	var payload messaging.RequestDiagnosisPayload
	require.NoError(t, requested.Message.DecodePayload(&payload))
	assert.Equal(t, "vehicle-1", payload.VehicleID)
	assert.Equal(t, "engine noise", payload.Notes)
}

func TestStart_ShouldReturnErrorWhenSaveFails(t *testing.T) {
	s := newScenario(t)
	s.store.saveErr = errDatabase

	_, err := s.orchestrator.Start(ctx, s.order)

	assert.ErrorIs(t, err, errDatabase)
}

func TestStart_ShouldNotSetDeadlineWhenTimeoutIsDisabled(t *testing.T) {
	s := newScenario(t)
	orchestrator := saga.NewOrchestrator(s.store, s.orderRepo, s.customerRepo, s.notifier, 0)

	created, err := orchestrator.Start(ctx, s.order)

	require.NoError(t, err)
	assert.Nil(t, created.Deadline())
}

func TestHandle_ShouldCompleteSagaAlongTheHappyPath(t *testing.T) {
	s := startedScenario(t)

	s.untilExecutionRequested(t)
	s.mustReply(t, messaging.TypeExecutionStarted, nil)
	s.mustReply(t, messaging.TypeExecutionFinished, nil)

	assert.Equal(t, valueobject.SagaStatusCompleted, s.store.saga.Status())
	assert.Equal(t, valueobject.OrderStatusFinished, s.order.Status())
	assert.Equal(t, []string{
		messaging.TypeRequestDiagnosis,
		messaging.TypeGenerateBudget,
		messaging.TypeRequestPayment,
		messaging.TypeStartExecution,
	}, s.store.publishedTypes())
	assert.Equal(t, []string{"in_diagnosis", "waiting_approval", "waiting_payment", "in_execution", "finished"}, s.notifier.notified)
}

func TestHandle_ShouldSendFrozenDiagnosisAndCustomerToBilling(t *testing.T) {
	s := startedScenario(t)

	s.mustReply(t, messaging.TypeDiagnosisCompleted, diagnosis)

	generate := s.store.lastPublished(t)
	assert.Equal(t, messaging.DestinationBillingCommands, generate.Destination)
	var payload messaging.GenerateBudgetPayload
	require.NoError(t, generate.Message.DecodePayload(&payload))
	assert.Equal(t, s.customer.ID(), payload.CustomerID)
	assert.Equal(t, "Maria Silva", payload.CustomerName)
	assert.Equal(t, "maria@example.com", payload.CustomerEmail)
	assert.Equal(t, diagnosis.Services, payload.Services)
	assert.Equal(t, diagnosis.Parts, payload.Parts)
	assert.True(t, s.store.saga.IsAt(valueobject.SagaStepBudgetApproval))
	assert.Equal(t, valueobject.OrderStatusInDiagnosis, s.order.Status())
}

func TestHandle_ShouldAttachBudgetAndPaymentToOrder(t *testing.T) {
	s := startedScenario(t)

	s.untilWaitingPayment(t)

	assert.Equal(t, "budget-1", s.order.BudgetID())
	assert.Equal(t, 180.0, s.order.TotalAmount())
	assert.Equal(t, "payment-1", s.order.PaymentID())
	assert.Equal(t, "https://checkout.example.com/pref-1", s.order.CheckoutURL())
	assert.Equal(t, valueobject.OrderStatusWaitingPayment, s.order.Status())
}

func TestHandle_ShouldCancelWithoutCompensationWhenDiagnosisFails(t *testing.T) {
	s := startedScenario(t)

	s.mustReply(t, messaging.TypeDiagnosisFailed, messaging.ReasonPayload{Reason: "vehicle not delivered"})

	assert.Equal(t, valueobject.SagaStatusCompensated, s.store.saga.Status())
	assert.True(t, s.order.IsCancelled())
	assert.Equal(t, "vehicle not delivered", s.order.CancellationReason())
	assert.Equal(t, []string{messaging.TypeRequestDiagnosis}, s.store.publishedTypes())
}

func TestHandle_ShouldCancelWorkOrderWhenBudgetIsRejected(t *testing.T) {
	s := startedScenario(t)
	s.untilWaitingApproval(t)

	s.mustReply(t, messaging.TypeBudgetRejected, messaging.BudgetDecisionPayload{BudgetID: "budget-1", Reason: "too expensive"})

	assert.True(t, s.store.saga.IsCompensating())
	assert.True(t, s.order.IsCancelled())
	compensation := s.store.lastPublished(t)
	assert.Equal(t, messaging.TypeCancelWorkOrder, compensation.Message.Type)
	assert.Equal(t, messaging.DestinationExecutionCommands, compensation.Destination)
	var payload messaging.ReasonPayload
	require.NoError(t, compensation.Message.DecodePayload(&payload))
	assert.Equal(t, "too expensive", payload.Reason)

	s.mustReply(t, messaging.TypeWorkOrderCancelled, nil)

	assert.Equal(t, valueobject.SagaStatusCompensated, s.store.saga.Status())
}

func TestHandle_ShouldCancelBudgetThenWorkOrderWhenPaymentFails(t *testing.T) {
	s := startedScenario(t)
	s.untilWaitingPayment(t)

	s.mustReply(t, messaging.TypePaymentFailed, messaging.PaymentOutcomePayload{PaymentID: "payment-1", Reason: "card declined"})
	s.mustReply(t, messaging.TypeBudgetCancelled, nil)
	s.mustReply(t, messaging.TypeWorkOrderCancelled, nil)

	assert.Equal(t, valueobject.SagaStatusCompensated, s.store.saga.Status())
	assert.Equal(t, "card declined", s.order.CancellationReason())
	assert.Equal(t, []string{messaging.TypeCancelBudget, messaging.TypeCancelWorkOrder}, s.store.publishedTypes()[3:])
}

func TestHandle_ShouldRefundThenCancelEverythingWhenExecutionIsRejected(t *testing.T) {
	s := startedScenario(t)
	s.untilExecutionRequested(t)

	s.mustReply(t, messaging.TypeExecutionRejected, messaging.ReasonPayload{Reason: "insufficient stock quantity"})
	s.mustReply(t, messaging.TypePaymentRefunded, nil)
	s.mustReply(t, messaging.TypeBudgetCancelled, nil)
	s.mustReply(t, messaging.TypeWorkOrderCancelled, nil)

	assert.Equal(t, valueobject.SagaStatusCompensated, s.store.saga.Status())
	assert.True(t, s.order.IsCancelled())
	assert.Equal(t, []string{
		messaging.TypeRefundPayment, messaging.TypeCancelBudget, messaging.TypeCancelWorkOrder,
	}, s.store.publishedTypes()[4:])
	assert.Equal(t, "cancelled", s.notifier.notified[len(s.notifier.notified)-1])
}

func TestHandle_ShouldUseReplyTypeAsReasonWhenFailureHasNone(t *testing.T) {
	s := startedScenario(t)

	s.mustReply(t, messaging.TypeDiagnosisFailed, nil)

	assert.Equal(t, messaging.TypeDiagnosisFailed, s.order.CancellationReason())
}

func TestHandle_ShouldIgnoreDuplicatedReply(t *testing.T) {
	s := startedScenario(t)
	s.untilWaitingApproval(t)
	s.mustReply(t, messaging.TypeBudgetApproved, nil)
	published := len(s.store.published)

	s.mustReply(t, messaging.TypeBudgetApproved, nil)
	s.mustReply(t, messaging.TypeDiagnosisCompleted, diagnosis)

	assert.Len(t, s.store.published, published)
	assert.True(t, s.store.saga.IsAt(valueobject.SagaStepPayment))
}

func TestHandle_ShouldIgnoreCompensationConfirmedOutOfOrder(t *testing.T) {
	s := startedScenario(t)
	s.untilWaitingPayment(t)
	s.mustReply(t, messaging.TypePaymentFailed, messaging.PaymentOutcomePayload{Reason: "card declined"})

	s.mustReply(t, messaging.TypeWorkOrderCancelled, nil)

	assert.True(t, s.store.saga.IsCompensating())
	assert.Len(t, s.store.saga.PendingCompensations(), 2)
}

func TestHandle_ShouldIgnoreProgressRepliesAfterSagaFails(t *testing.T) {
	s := startedScenario(t)
	s.mustReply(t, messaging.TypeDiagnosisFailed, messaging.ReasonPayload{Reason: "vehicle not delivered"})

	for _, replyType := range []string{
		messaging.TypeDiagnosisStarted, messaging.TypeBudgetGenerated, messaging.TypePaymentRequested,
		messaging.TypePaymentConfirmed, messaging.TypeExecutionStarted, messaging.TypeExecutionFinished,
		messaging.TypeBudgetRejected,
	} {
		s.mustReply(t, replyType, nil)
	}

	assert.Equal(t, valueobject.SagaStatusCompensated, s.store.saga.Status())
	assert.True(t, s.order.IsCancelled())
}

func TestHandle_ShouldTolerateStatusRepliesArrivingOutOfOrder(t *testing.T) {
	s := startedScenario(t)
	s.mustReply(t, messaging.TypeDiagnosisCompleted, diagnosis)

	s.mustReply(t, messaging.TypeDiagnosisStarted, nil)

	assert.Equal(t, valueobject.OrderStatusInDiagnosis, s.order.Status())
	assert.True(t, s.store.saga.IsAt(valueobject.SagaStepBudgetApproval))
}

func TestHandle_ShouldReturnErrorForUnknownSaga(t *testing.T) {
	s := startedScenario(t)
	message, err := messaging.NewMessage(messaging.TypeDiagnosisStarted, "unknown-saga", s.order.ID(), nil)
	require.NoError(t, err)

	err = s.orchestrator.Handle(ctx, message)

	assert.ErrorIs(t, err, repository.ErrSagaNotFound)
}

func TestHandle_ShouldReturnErrorForUnknownReplyType(t *testing.T) {
	s := startedScenario(t)

	err := s.reply(t, "SomethingElse", nil)

	assert.ErrorIs(t, err, messaging.ErrUnknownMessageType)
}

func TestHandle_ShouldReturnErrorWhenOrderIsMissing(t *testing.T) {
	s := newScenario(t)
	orderRepo := mocks.NewMockServiceOrderRepository(t)
	orderRepo.On("FindByID", mock.Anything, s.order.ID()).Return(nil, repository.ErrServiceOrderNotFound)
	orchestrator := saga.NewOrchestrator(s.store, orderRepo, s.customerRepo, s.notifier, time.Hour)
	_, err := orchestrator.Start(ctx, s.order)
	require.NoError(t, err)
	message, err := messaging.NewMessage(messaging.TypeDiagnosisStarted, s.store.saga.ID(), s.order.ID(), nil)
	require.NoError(t, err)

	err = orchestrator.Handle(ctx, message)

	assert.ErrorIs(t, err, repository.ErrServiceOrderNotFound)
}

func TestHandle_ShouldReturnErrorForInvalidPayload(t *testing.T) {
	replyTypes := []string{
		messaging.TypeDiagnosisCompleted,
		messaging.TypeBudgetGenerated,
		messaging.TypePaymentRequested,
		messaging.TypeDiagnosisFailed,
	}

	for _, replyType := range replyTypes {
		t.Run(replyType, func(t *testing.T) {
			s := startedScenario(t)
			message, err := messaging.NewMessage(replyType, s.store.saga.ID(), s.order.ID(), nil)
			require.NoError(t, err)
			message.Payload = []byte(`[]`)

			err = s.orchestrator.Handle(ctx, message)

			assert.ErrorIs(t, err, messaging.ErrMessagePayloadInvalid)
		})
	}
}

func TestHandle_ShouldReturnErrorWhenCustomerIsMissing(t *testing.T) {
	s := newScenario(t)
	customerRepo := mocks.NewMockCustomerRepository(t)
	customerRepo.On("FindByID", s.customer.ID()).Return(nil, repository.ErrCustomerNotFound)
	orchestrator := saga.NewOrchestrator(s.store, s.orderRepo, customerRepo, s.notifier, time.Hour)
	_, err := orchestrator.Start(ctx, s.order)
	require.NoError(t, err)
	message, err := messaging.NewMessage(messaging.TypeDiagnosisCompleted, s.store.saga.ID(), s.order.ID(), diagnosis)
	require.NoError(t, err)

	err = orchestrator.Handle(ctx, message)

	assert.ErrorIs(t, err, repository.ErrCustomerNotFound)
}

func TestHandle_ShouldReturnErrorAndSkipNotificationWhenSaveFails(t *testing.T) {
	s := startedScenario(t)
	s.store.saveErr = repository.ErrSagaConflict

	err := s.reply(t, messaging.TypeDiagnosisStarted, nil)

	assert.ErrorIs(t, err, repository.ErrSagaConflict)
	assert.Empty(t, s.notifier.notified)
}

func TestCancel_ShouldCompensateAccordingToCurrentStep(t *testing.T) {
	cases := []struct {
		name     string
		advance  func(t *testing.T, s *scenario)
		expected string
		pending  int
	}{
		{"diagnosis", func(t *testing.T, s *scenario) {}, messaging.TypeCancelWorkOrder, 1},
		{"budget approval", func(t *testing.T, s *scenario) { s.untilWaitingApproval(t) }, messaging.TypeCancelBudget, 2},
		{"payment", func(t *testing.T, s *scenario) { s.untilWaitingPayment(t) }, messaging.TypeRefundPayment, 3},
		{"execution", func(t *testing.T, s *scenario) { s.untilExecutionRequested(t) }, messaging.TypeRefundPayment, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := startedScenario(t)
			tc.advance(t, s)

			order, err := s.orchestrator.Cancel(ctx, s.order.ID(), "customer gave up")

			require.NoError(t, err)
			assert.True(t, order.IsCancelled())
			assert.Equal(t, "customer gave up", order.CancellationReason())
			assert.Equal(t, tc.expected, s.store.lastPublished(t).Message.Type)
			assert.Len(t, s.store.saga.PendingCompensations(), tc.pending)
		})
	}
}

func TestCancel_ShouldReturnErrorWhenOrderIsAlreadyFinished(t *testing.T) {
	s := startedScenario(t)
	s.untilExecutionRequested(t)
	s.mustReply(t, messaging.TypeExecutionFinished, nil)

	_, err := s.orchestrator.Cancel(ctx, s.order.ID(), "too late")

	assert.ErrorIs(t, err, entity.ErrServiceOrderInvalidTransition)
}

func TestCancel_ShouldReturnErrorWhenReasonIsMissing(t *testing.T) {
	s := startedScenario(t)

	_, err := s.orchestrator.Cancel(ctx, s.order.ID(), "")

	assert.ErrorIs(t, err, entity.ErrServiceOrderCancellationReasonRequired)
}

func TestCancel_ShouldReturnErrorWhenOrderHasNoSaga(t *testing.T) {
	s := newScenario(t)

	_, err := s.orchestrator.Cancel(ctx, s.order.ID(), "customer gave up")

	assert.ErrorIs(t, err, repository.ErrSagaNotFound)
}

func TestCancel_ShouldReturnErrorWhenSaveFails(t *testing.T) {
	s := startedScenario(t)
	s.store.saveErr = errDatabase

	_, err := s.orchestrator.Cancel(ctx, s.order.ID(), "customer gave up")

	assert.ErrorIs(t, err, errDatabase)
}

func TestExpireOverdue_ShouldCancelOverdueSagasWithTimeoutReason(t *testing.T) {
	s := startedScenario(t)
	s.store.overdue = []*entity.Saga{s.store.saga}

	expired, err := s.orchestrator.ExpireOverdue(ctx)

	require.NoError(t, err)
	assert.Equal(t, 1, expired)
	assert.True(t, s.order.IsCancelled())
	assert.Equal(t, "saga step timed out", s.order.CancellationReason())
}

func TestExpireOverdue_ShouldDoNothingWithoutOverdueSagas(t *testing.T) {
	s := startedScenario(t)

	expired, err := s.orchestrator.ExpireOverdue(ctx)

	require.NoError(t, err)
	assert.Zero(t, expired)
	assert.False(t, s.order.IsCancelled())
}

func TestExpireOverdue_ShouldStopOnFirstFailure(t *testing.T) {
	s := startedScenario(t)
	s.store.overdue = []*entity.Saga{s.store.saga}
	s.store.saveErr = errDatabase

	expired, err := s.orchestrator.ExpireOverdue(ctx)

	assert.ErrorIs(t, err, errDatabase)
	assert.Zero(t, expired)
}
