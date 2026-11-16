package entity

import "errors"

var (
	ErrCustomerNameRequired  = errors.New("customer name is required")
	ErrCustomerPhoneRequired = errors.New("customer phone is required")
	ErrCustomerEmailRequired = errors.New("customer email is required")
	ErrInvalidDocument       = errors.New("document must be a valid CPF or CNPJ")

	ErrVehicleBrandRequired    = errors.New("vehicle brand is required")
	ErrVehicleModelRequired    = errors.New("vehicle model is required")
	ErrVehicleInvalidYear      = errors.New("vehicle year is invalid")
	ErrVehicleCustomerRequired = errors.New("vehicle customer id is required")

	ErrServiceOrderCustomerRequired           = errors.New("service order customer id is required")
	ErrServiceOrderVehicleRequired            = errors.New("service order vehicle id is required")
	ErrServiceOrderVehicleNotOwnedByCustomer  = errors.New("vehicle does not belong to the specified customer")
	ErrServiceOrderInvalidTransition          = errors.New("service order status transition not allowed")
	ErrServiceOrderCancellationReasonRequired = errors.New("service order cancellation reason is required")

	ErrSagaOrderRequired  = errors.New("saga service order id is required")
	ErrSagaNotRunning     = errors.New("saga is not running")
	ErrSagaReasonRequired = errors.New("saga failure reason is required")
)
