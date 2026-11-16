package valueobject

type SagaStatus string

const (
	SagaStatusRunning      SagaStatus = "running"
	SagaStatusCompensating SagaStatus = "compensating"
	SagaStatusCompleted    SagaStatus = "completed"
	SagaStatusCompensated  SagaStatus = "compensated"
)

func (s SagaStatus) String() string {
	return string(s)
}

type SagaStep string

const (
	SagaStepDiagnosis      SagaStep = "diagnosis"
	SagaStepBudgetApproval SagaStep = "budget_approval"
	SagaStepPayment        SagaStep = "payment"
	SagaStepExecution      SagaStep = "execution"
)

var sagaStepOrder = map[SagaStep]int{
	SagaStepDiagnosis:      1,
	SagaStepBudgetApproval: 2,
	SagaStepPayment:        3,
	SagaStepExecution:      4,
}

func (s SagaStep) IsBefore(next SagaStep) bool {
	return sagaStepOrder[s] < sagaStepOrder[next]
}

func (s SagaStep) String() string {
	return string(s)
}

type SagaLogKind string

const (
	SagaLogKindCommand      SagaLogKind = "command"
	SagaLogKindReply        SagaLogKind = "reply"
	SagaLogKindCompensation SagaLogKind = "compensation"
	SagaLogKindFailure      SagaLogKind = "failure"
	SagaLogKindOutcome      SagaLogKind = "outcome"
)

func (k SagaLogKind) String() string {
	return string(k)
}
