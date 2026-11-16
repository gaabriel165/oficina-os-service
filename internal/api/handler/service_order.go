package handler

import (
	"net/http"

	"github.com/gabrielcamargo/oficina-api/internal/api/dto"
	"github.com/gabrielcamargo/oficina-api/internal/api/middleware"
	"github.com/gabrielcamargo/oficina-api/internal/api/response"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase/serviceorder"
	"github.com/gabrielcamargo/oficina-api/internal/domain/entity"
	"github.com/gin-gonic/gin"
)

type ServiceOrderHandler struct {
	createUC  *serviceorder.CreateServiceOrderUseCase
	cancelUC  *serviceorder.CancelServiceOrderUseCase
	deliverUC *serviceorder.DeliverVehicleUseCase
	getUC     *serviceorder.GetServiceOrderUseCase
	listUC    *serviceorder.ListServiceOrdersUseCase
	historyUC *serviceorder.GetServiceOrderHistoryUseCase
	sagaUC    *serviceorder.GetServiceOrderSagaUseCase
}

func NewServiceOrderHandler(
	createUC *serviceorder.CreateServiceOrderUseCase,
	cancelUC *serviceorder.CancelServiceOrderUseCase,
	deliverUC *serviceorder.DeliverVehicleUseCase,
	getUC *serviceorder.GetServiceOrderUseCase,
	listUC *serviceorder.ListServiceOrdersUseCase,
	historyUC *serviceorder.GetServiceOrderHistoryUseCase,
	sagaUC *serviceorder.GetServiceOrderSagaUseCase,
) *ServiceOrderHandler {
	return &ServiceOrderHandler{
		createUC:  createUC,
		cancelUC:  cancelUC,
		deliverUC: deliverUC,
		getUC:     getUC,
		listUC:    listUC,
		historyUC: historyUC,
		sagaUC:    sagaUC,
	}
}

func (h *ServiceOrderHandler) RegisterOperatorRoutes(router *gin.RouterGroup) {
	router.POST("/service-orders", h.Create)
	router.PATCH("/service-orders/:id/cancel", h.Cancel)
	router.PATCH("/service-orders/:id/deliver", h.DeliverVehicle)
}

func (h *ServiceOrderHandler) RegisterSharedRoutes(router *gin.RouterGroup) {
	router.GET("/service-orders", h.List)
	router.GET("/service-orders/:id", h.Get)
	router.GET("/service-orders/:id/history", h.GetHistory)
	router.GET("/service-orders/:id/saga", h.GetSaga)
}

func (h *ServiceOrderHandler) RegisterPublicRoutes(router *gin.RouterGroup) {
	router.GET("/service-orders/:id/status", h.GetStatus)
}

func trackingRequesterFrom(c *gin.Context) serviceorder.TrackingRequester {
	subject, role := middleware.Principal(c)
	return serviceorder.TrackingRequester{ID: subject, IsCustomer: role == middleware.RoleCustomer}
}

// GetStatus godoc
// @Summary      Get the current status of a service order (public)
// @Tags         service-orders
// @Produce      json
// @Param        id  path      string  true  "Service Order ID"
// @Success      200  {object}  dto.ServiceOrderStatusResponse
// @Failure      404  {object}  response.ErrorResponse
// @Router       /service-orders/{id}/status [get]
func (h *ServiceOrderHandler) GetStatus(c *gin.Context) {
	result, err := h.getUC.Execute(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ToServiceOrderStatusResponse(result))
}

// Create godoc
// @Summary      Open a service order and start its saga
// @Description  The order is created as received and the diagnosis is requested from the Execution Service. Services and parts are added by the mechanic during diagnosis.
// @Tags         service-orders
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      dto.CreateServiceOrderRequest  true  "Service order data"
// @Success      201   {object}  dto.ServiceOrderResponse
// @Failure      400   {object}  response.ErrorResponse
// @Failure      404   {object}  response.ErrorResponse
// @Failure      422   {object}  response.ErrorResponse
// @Router       /service-orders [post]
func (h *ServiceOrderHandler) Create(c *gin.Context) {
	var req dto.CreateServiceOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.createUC.Execute(c.Request.Context(), serviceorder.CreateServiceOrderInput{
		CustomerID: req.CustomerID,
		VehicleID:  req.VehicleID,
		Notes:      req.Notes,
	})
	respondCreatedOrder(c, result, err)
}

// Get godoc
// @Summary      Get a service order by ID
// @Description  Operators can read any order; customers authenticated by CPF can only read their own.
// @Tags         service-orders
// @Security     BearerAuth
// @Produce      json
// @Param        id  path      string  true  "Service Order ID"
// @Success      200  {object}  dto.ServiceOrderResponse
// @Failure      403  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Router       /service-orders/{id} [get]
func (h *ServiceOrderHandler) Get(c *gin.Context) {
	subject, role := middleware.Principal(c)
	var result *entity.ServiceOrder
	var err error
	if role == middleware.RoleCustomer {
		result, err = h.getUC.ExecuteForCustomer(c.Request.Context(), c.Param("id"), subject)
	} else {
		result, err = h.getUC.Execute(c.Request.Context(), c.Param("id"))
	}
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ToServiceOrderResponse(result))
}

// List godoc
// @Summary      List active service orders
// @Description  Operators see every active order sorted by urgency; customers authenticated by CPF see only their own.
// @Tags         service-orders
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}   dto.ServiceOrderResponse
// @Router       /service-orders [get]
func (h *ServiceOrderHandler) List(c *gin.Context) {
	subject, role := middleware.Principal(c)
	var results []*entity.ServiceOrder
	var err error
	if role == middleware.RoleCustomer {
		results, err = h.listUC.ExecuteForCustomer(c.Request.Context(), subject)
	} else {
		results, err = h.listUC.Execute(c.Request.Context())
	}
	if err != nil {
		response.Error(c, err)
		return
	}

	responses := make([]dto.ServiceOrderResponse, 0, len(results))
	for _, r := range results {
		responses = append(responses, dto.ToServiceOrderResponse(r))
	}
	c.JSON(http.StatusOK, responses)
}

// GetHistory godoc
// @Summary      Get the status history of a service order
// @Tags         service-orders
// @Security     BearerAuth
// @Produce      json
// @Param        id  path      string  true  "Service Order ID"
// @Success      200  {array}   dto.StatusChangeResponse
// @Failure      403  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Router       /service-orders/{id}/history [get]
func (h *ServiceOrderHandler) GetHistory(c *gin.Context) {
	history, err := h.historyUC.Execute(c.Request.Context(), c.Param("id"), trackingRequesterFrom(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ToStatusHistoryResponse(history))
}

// GetSaga godoc
// @Summary      Get the saga of a service order: current step, commands, replies and compensations
// @Tags         service-orders
// @Security     BearerAuth
// @Produce      json
// @Param        id  path      string  true  "Service Order ID"
// @Success      200  {object}  dto.SagaResponse
// @Failure      403  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Router       /service-orders/{id}/saga [get]
func (h *ServiceOrderHandler) GetSaga(c *gin.Context) {
	saga, err := h.sagaUC.Execute(c.Request.Context(), c.Param("id"), trackingRequesterFrom(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ToSagaResponse(saga))
}

// Cancel godoc
// @Summary      Cancel a service order, compensating the steps already completed
// @Tags         service-orders
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path      string                         true  "Service Order ID"
// @Param        body  body      dto.CancelServiceOrderRequest  true  "Cancellation reason"
// @Success      200   {object}  dto.ServiceOrderResponse
// @Failure      400   {object}  response.ErrorResponse
// @Failure      404   {object}  response.ErrorResponse
// @Failure      422   {object}  response.ErrorResponse
// @Router       /service-orders/{id}/cancel [patch]
func (h *ServiceOrderHandler) Cancel(c *gin.Context) {
	var req dto.CancelServiceOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.cancelUC.Execute(c.Request.Context(), serviceorder.CancelServiceOrderInput{
		OrderID: c.Param("id"),
		Reason:  req.Reason,
	})
	if err != nil {
		failOrder(c, "cancel", c.Param("id"), err)
		return
	}
	c.JSON(http.StatusOK, dto.ToServiceOrderResponse(result))
}

// DeliverVehicle godoc
// @Summary      Deliver the vehicle to the customer
// @Tags         service-orders
// @Security     BearerAuth
// @Produce      json
// @Param        id  path      string  true  "Service Order ID"
// @Success      200  {object}  dto.ServiceOrderResponse
// @Failure      404  {object}  response.ErrorResponse
// @Failure      422  {object}  response.ErrorResponse
// @Router       /service-orders/{id}/deliver [patch]
func (h *ServiceOrderHandler) DeliverVehicle(c *gin.Context) {
	result, err := h.deliverUC.Execute(c.Request.Context(), c.Param("id"))
	respondOrderTransition(c, "deliver", result, err)
}
