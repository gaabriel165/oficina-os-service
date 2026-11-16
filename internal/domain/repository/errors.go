package repository

import "errors"

var (
	ErrCustomerNotFound     = errors.New("customer not found")
	ErrVehicleNotFound      = errors.New("vehicle not found")
	ErrServiceOrderNotFound = errors.New("service order not found")
	ErrUserNotFound         = errors.New("user not found")
	ErrSagaNotFound         = errors.New("saga not found")
	ErrSagaConflict         = errors.New("saga was changed by another process")
)
