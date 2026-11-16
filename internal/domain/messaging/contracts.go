package messaging

const (
	TypeRequestDiagnosis = "RequestDiagnosis"
	TypeStartExecution   = "StartExecution"
	TypeCancelWorkOrder  = "CancelWorkOrder"

	TypeGenerateBudget = "GenerateBudget"
	TypeRequestPayment = "RequestPayment"
	TypeCancelBudget   = "CancelBudget"
	TypeRefundPayment  = "RefundPayment"

	TypeDiagnosisStarted   = "DiagnosisStarted"
	TypeDiagnosisCompleted = "DiagnosisCompleted"
	TypeDiagnosisFailed    = "DiagnosisFailed"
	TypeExecutionStarted   = "ExecutionStarted"
	TypeExecutionRejected  = "ExecutionRejected"
	TypeExecutionFinished  = "ExecutionFinished"
	TypeWorkOrderCancelled = "WorkOrderCancelled"

	TypeBudgetGenerated  = "BudgetGenerated"
	TypeBudgetApproved   = "BudgetApproved"
	TypeBudgetRejected   = "BudgetRejected"
	TypePaymentRequested = "PaymentRequested"
	TypePaymentConfirmed = "PaymentConfirmed"
	TypePaymentFailed    = "PaymentFailed"
	TypeBudgetCancelled  = "BudgetCancelled"
	TypePaymentRefunded  = "PaymentRefunded"
)

type ServiceLine struct {
	ServiceID  string  `json:"service_id"`
	Name       string  `json:"name"`
	LaborPrice float64 `json:"labor_price"`
}

type PartLine struct {
	PartID    string  `json:"part_id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type RequestDiagnosisPayload struct {
	CustomerID string `json:"customer_id"`
	VehicleID  string `json:"vehicle_id"`
	Notes      string `json:"notes"`
}

type ReasonPayload struct {
	Reason string `json:"reason"`
}

type GenerateBudgetPayload struct {
	CustomerID    string        `json:"customer_id"`
	CustomerName  string        `json:"customer_name"`
	CustomerEmail string        `json:"customer_email"`
	Services      []ServiceLine `json:"services"`
	Parts         []PartLine    `json:"parts"`
}

type WorkOrderPayload struct {
	WorkOrderID string `json:"work_order_id"`
}

type DiagnosisCompletedPayload struct {
	WorkOrderID string        `json:"work_order_id"`
	Services    []ServiceLine `json:"services"`
	Parts       []PartLine    `json:"parts"`
}

type BudgetGeneratedPayload struct {
	BudgetID    string  `json:"budget_id"`
	TotalAmount float64 `json:"total_amount"`
}

type BudgetDecisionPayload struct {
	BudgetID string `json:"budget_id"`
	Reason   string `json:"reason,omitempty"`
}

type PaymentRequestedPayload struct {
	PaymentID   string `json:"payment_id"`
	CheckoutURL string `json:"checkout_url"`
}

type PaymentOutcomePayload struct {
	PaymentID string `json:"payment_id"`
	Reason    string `json:"reason,omitempty"`
}
