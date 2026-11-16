package handler

import (
	"net/http"

	"github.com/gabrielcamargo/oficina-api/internal/api/dto"
	"github.com/gabrielcamargo/oficina-api/internal/api/response"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase/customer"
	"github.com/gin-gonic/gin"
)

type InternalCustomerHandler struct {
	findByDocumentUC *customer.FindCustomerByDocumentUseCase
}

func NewInternalCustomerHandler(findByDocumentUC *customer.FindCustomerByDocumentUseCase) *InternalCustomerHandler {
	return &InternalCustomerHandler{findByDocumentUC: findByDocumentUC}
}

func (h *InternalCustomerHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/customers/by-document/:document", h.FindByDocument)
}

// FindByDocument godoc
// @Summary      Find a customer by CPF or CNPJ (service-to-service)
// @Description  Used by the authentication Lambda instead of reading the customers table. Requires the X-Internal-Secret header and is not routed by the API Gateway.
// @Tags         internal
// @Produce      json
// @Param        document           path      string  true  "CPF or CNPJ, digits only or formatted"
// @Param        X-Internal-Secret  header    string  true  "Shared service secret"
// @Success      200  {object}  dto.CustomerIdentityResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Router       /internal/customers/by-document/{document} [get]
func (h *InternalCustomerHandler) FindByDocument(c *gin.Context) {
	result, err := h.findByDocumentUC.Execute(c.Param("document"))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ToCustomerIdentityResponse(result))
}
