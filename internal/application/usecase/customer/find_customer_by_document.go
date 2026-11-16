package customer

import (
	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gabrielcamargo/oficina-api/internal/domain/repository"
	"github.com/gabrielcamargo/oficina-api/internal/utils"
)

type FindCustomerByDocumentUseCase struct {
	customerRepo repository.CustomerRepository
}

func NewFindCustomerByDocumentUseCase(customerRepo repository.CustomerRepository) *FindCustomerByDocumentUseCase {
	return &FindCustomerByDocumentUseCase{customerRepo: customerRepo}
}

func (u *FindCustomerByDocumentUseCase) Execute(document string) (*entity.Customer, error) {
	return u.customerRepo.FindByDocument(utils.CleanNumericString(document))
}
