package customer_test

import (
	"testing"

	"github.com/gabrielcamargo/oficina-api/internal/application/mocks"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase/customer"
	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/repository"
	"github.com/stretchr/testify/assert"
)

func TestFindCustomerByDocument_ShouldSearchUsingDigitsOnly(t *testing.T) {
	customerRepo := mocks.NewMockCustomerRepository(t)
	existing, _ := entity.NewCustomer("Maria Silva", "529.982.247-25", "11999999999", "maria@example.com")
	customerRepo.On("FindByDocument", "52998224725").Return(existing, nil)

	result, err := customer.NewFindCustomerByDocumentUseCase(customerRepo).Execute("529.982.247-25")

	assert.NoError(t, err)
	assert.Equal(t, existing.ID(), result.ID())
}

func TestFindCustomerByDocument_ShouldReturnErrorWhenNotFound(t *testing.T) {
	customerRepo := mocks.NewMockCustomerRepository(t)
	customerRepo.On("FindByDocument", "52998224725").Return(nil, repository.ErrCustomerNotFound)

	_, err := customer.NewFindCustomerByDocumentUseCase(customerRepo).Execute("52998224725")

	assert.ErrorIs(t, err, repository.ErrCustomerNotFound)
}
