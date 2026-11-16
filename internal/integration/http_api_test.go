package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gabrielcamargo/oficina-api/configs"
	"github.com/gabrielcamargo/oficina-api/internal/api"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiJWTSecret      = "integration-jwt-secret"
	apiInternalSecret = "integration-internal-secret"
)

type apiClient struct {
	t      *testing.T
	server *api.Server
	token  string
}

func newAPIClient(t *testing.T) *apiClient {
	t.Helper()
	gin.SetMode(gin.TestMode)
	server := api.NewServer(&configs.Config{
		ServerPort:        "0",
		ServiceName:       "oficina-os-service",
		JWTSecret:         apiJWTSecret,
		InternalAPISecret: apiInternalSecret,
		SagaStepTimeout:   time.Hour,
	}, newTestDB(t))

	client := &apiClient{t: t, server: server}
	client.token = client.registerAndLogin("operator@workshop.com", "secret123")
	return client
}

func (c *apiClient) do(method, path, token, body string, headers ...string) *httptest.ResponseRecorder {
	c.t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for index := 0; index+1 < len(headers); index += 2 {
		request.Header.Set(headers[index], headers[index+1])
	}
	recorder := httptest.NewRecorder()
	c.server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func (c *apiClient) asOperator(method, path, body string) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.do(method, path, c.token, body)
}

func (c *apiClient) registerAndLogin(email, password string) string {
	c.t.Helper()
	credentials := `{"email":"` + email + `","password":"` + password + `"}`
	require.Equal(c.t, http.StatusCreated, c.do(http.MethodPost, "/api/v1/auth/register", "", credentials).Code)
	login := c.do(http.MethodPost, "/api/v1/auth/login", "", credentials)
	require.Equal(c.t, http.StatusOK, login.Code, login.Body.String())
	return field(c.t, login, "token")
}

func (c *apiClient) customerToken(customerID string) string {
	c.t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  customerID,
		"role": "customer",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(apiJWTSecret))
	require.NoError(c.t, err)
	return signed
}

func (c *apiClient) createCustomer(name, document, email string) string {
	c.t.Helper()
	recorder := c.asOperator(http.MethodPost, "/api/v1/customers",
		`{"name":"`+name+`","document":"`+document+`","phone":"11999999999","email":"`+email+`"}`)
	require.Equal(c.t, http.StatusCreated, recorder.Code, recorder.Body.String())
	return field(c.t, recorder, "id")
}

func (c *apiClient) createVehicle(customerID, plate string) string {
	c.t.Helper()
	recorder := c.asOperator(http.MethodPost, "/api/v1/vehicles",
		`{"customer_id":"`+customerID+`","plate":"`+plate+`","brand":"Honda","model":"Civic","year":2022}`)
	require.Equal(c.t, http.StatusCreated, recorder.Code, recorder.Body.String())
	return field(c.t, recorder, "id")
}

func (c *apiClient) createOrder(customerID, vehicleID string) string {
	c.t.Helper()
	recorder := c.asOperator(http.MethodPost, "/api/v1/service-orders",
		`{"customer_id":"`+customerID+`","vehicle_id":"`+vehicleID+`","notes":"engine noise"}`)
	require.Equal(c.t, http.StatusCreated, recorder.Code, recorder.Body.String())
	return field(c.t, recorder, "id")
}

func field(t *testing.T, recorder *httptest.ResponseRecorder, name string) string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body), recorder.Body.String())
	value, ok := body[name].(string)
	require.True(t, ok, "field %q not found in %s", name, recorder.Body.String())
	return value
}

func TestAPI_ShouldServeHealthReadinessAndDocs(t *testing.T) {
	client := newAPIClient(t)

	assert.Equal(t, http.StatusOK, client.do(http.MethodGet, "/health", "", "").Code)
	assert.Equal(t, http.StatusOK, client.do(http.MethodGet, "/ready", "", "").Code)
	assert.Equal(t, http.StatusOK, client.do(http.MethodGet, "/swagger/index.html", "", "").Code)
}

func TestAPI_ShouldAuthenticateOperators(t *testing.T) {
	client := newAPIClient(t)

	t.Run("rejects duplicated registration", func(t *testing.T) {
		recorder := client.do(http.MethodPost, "/api/v1/auth/register", "", `{"email":"operator@workshop.com","password":"secret123"}`)
		assert.Equal(t, http.StatusConflict, recorder.Code)
	})

	t.Run("rejects invalid registration body", func(t *testing.T) {
		recorder := client.do(http.MethodPost, "/api/v1/auth/register", "", `{"email":"not-an-email","password":"1"}`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("rejects wrong password", func(t *testing.T) {
		recorder := client.do(http.MethodPost, "/api/v1/auth/login", "", `{"email":"operator@workshop.com","password":"wrong-password"}`)
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})

	t.Run("rejects invalid login body", func(t *testing.T) {
		recorder := client.do(http.MethodPost, "/api/v1/auth/login", "", `{}`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("rejects requests without token", func(t *testing.T) {
		recorder := client.do(http.MethodGet, "/api/v1/customers", "", "")
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})
}

func TestAPI_ShouldManageCustomers(t *testing.T) {
	client := newAPIClient(t)
	customerID := client.createCustomer("Maria Silva", "529.982.247-25", "maria@example.com")

	t.Run("rejects duplicated document", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPost, "/api/v1/customers",
			`{"name":"Outra Maria","document":"52998224725","phone":"11988888888","email":"outra@example.com"}`)
		assert.Equal(t, http.StatusConflict, recorder.Code)
	})

	t.Run("rejects invalid body", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPost, "/api/v1/customers", `{"name":"Sem documento"}`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("rejects invalid document", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPost, "/api/v1/customers",
			`{"name":"João","document":"111.111.111-11","phone":"11988888888","email":"joao@example.com"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	})

	t.Run("gets customer", func(t *testing.T) {
		recorder := client.asOperator(http.MethodGet, "/api/v1/customers/"+customerID, "")
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "Maria Silva", field(t, recorder, "name"))
	})

	t.Run("lists customers", func(t *testing.T) {
		recorder := client.asOperator(http.MethodGet, "/api/v1/customers", "")
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), customerID)
	})

	t.Run("updates customer", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPut, "/api/v1/customers/"+customerID,
			`{"name":"Maria Souza","phone":"11977777777","email":"maria.souza@example.com"}`)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "Maria Souza", field(t, recorder, "name"))
	})

	t.Run("rejects invalid update body", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPut, "/api/v1/customers/"+customerID, `{"name":"Maria"}`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("changes customer status", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPatch, "/api/v1/customers/"+customerID+"/status", `{"status":"inactive"}`)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "inactive", field(t, recorder, "status"))
	})

	t.Run("rejects unknown customer status", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPatch, "/api/v1/customers/"+customerID+"/status", `{"status":"sleeping"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	})

	t.Run("rejects status change without body", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPatch, "/api/v1/customers/"+customerID+"/status", `{}`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("returns not found for unknown customer", func(t *testing.T) {
		unknown := "/api/v1/customers/7d0a3c1b-2b1f-4f53-8a37-d1e6a7c49e55"
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodGet, unknown, "").Code)
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodPut, unknown, `{"name":"X","phone":"11977777777","email":"x@example.com"}`).Code)
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodPatch, unknown+"/status", `{"status":"inactive"}`).Code)
	})

	t.Run("deletes customer", func(t *testing.T) {
		assert.Equal(t, http.StatusNoContent, client.asOperator(http.MethodDelete, "/api/v1/customers/"+customerID, "").Code)
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodGet, "/api/v1/customers/"+customerID, "").Code)
	})
}

func TestAPI_ShouldManageVehicles(t *testing.T) {
	client := newAPIClient(t)
	customerID := client.createCustomer("Maria Silva", "529.982.247-25", "maria@example.com")
	vehicleID := client.createVehicle(customerID, "BRA2E19")

	t.Run("rejects duplicated plate", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPost, "/api/v1/vehicles",
			`{"customer_id":"`+customerID+`","plate":"BRA2E19","brand":"Fiat","model":"Uno","year":2015}`)
		assert.Equal(t, http.StatusConflict, recorder.Code)
	})

	t.Run("rejects invalid plate", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPost, "/api/v1/vehicles",
			`{"customer_id":"`+customerID+`","plate":"123","brand":"Fiat","model":"Uno","year":2015}`)
		assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	})

	t.Run("rejects invalid body", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPost, "/api/v1/vehicles", `{"plate":"BRA2E19"}`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("rejects vehicle for unknown customer", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPost, "/api/v1/vehicles",
			`{"customer_id":"7d0a3c1b-2b1f-4f53-8a37-d1e6a7c49e55","plate":"ABC1D23","brand":"Fiat","model":"Uno","year":2015}`)
		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("gets vehicle", func(t *testing.T) {
		recorder := client.asOperator(http.MethodGet, "/api/v1/vehicles/"+vehicleID, "")
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "BRA2E19", field(t, recorder, "plate"))
	})

	t.Run("lists vehicles", func(t *testing.T) {
		recorder := client.asOperator(http.MethodGet, "/api/v1/vehicles", "")
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), vehicleID)
	})

	t.Run("updates vehicle", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPut, "/api/v1/vehicles/"+vehicleID, `{"brand":"Honda","model":"Civic Touring","year":2023}`)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "Civic Touring", field(t, recorder, "model"))
	})

	t.Run("rejects invalid update body", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPut, "/api/v1/vehicles/"+vehicleID, `{"brand":"Honda"}`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("returns not found for unknown vehicle", func(t *testing.T) {
		unknown := "/api/v1/vehicles/7d0a3c1b-2b1f-4f53-8a37-d1e6a7c49e55"
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodGet, unknown, "").Code)
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodPut, unknown, `{"brand":"Honda","model":"Fit","year":2020}`).Code)
	})

	t.Run("deletes vehicle", func(t *testing.T) {
		assert.Equal(t, http.StatusNoContent, client.asOperator(http.MethodDelete, "/api/v1/vehicles/"+vehicleID, "").Code)
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodGet, "/api/v1/vehicles/"+vehicleID, "").Code)
	})
}

func TestAPI_ShouldOpenTrackAndCancelServiceOrders(t *testing.T) {
	client := newAPIClient(t)
	customerID := client.createCustomer("Maria Silva", "529.982.247-25", "maria@example.com")
	otherCustomerID := client.createCustomer("João Souza", "111.444.777-35", "joao@example.com")
	vehicleID := client.createVehicle(customerID, "BRA2E19")
	otherVehicleID := client.createVehicle(otherCustomerID, "RIO2A18")
	orderID := client.createOrder(customerID, vehicleID)
	otherOrderID := client.createOrder(otherCustomerID, otherVehicleID)
	ownerToken := client.customerToken(customerID)
	orderPath := "/api/v1/service-orders/" + orderID

	t.Run("rejects vehicle of another customer", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPost, "/api/v1/service-orders",
			`{"customer_id":"`+customerID+`","vehicle_id":"`+otherVehicleID+`"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	})

	t.Run("rejects invalid body", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPost, "/api/v1/service-orders", `{"notes":"no ids"}`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("forbids customers from opening orders", func(t *testing.T) {
		recorder := client.do(http.MethodPost, "/api/v1/service-orders", ownerToken,
			`{"customer_id":"`+customerID+`","vehicle_id":"`+vehicleID+`"}`)
		assert.Equal(t, http.StatusForbidden, recorder.Code)
	})

	t.Run("gets order as operator", func(t *testing.T) {
		recorder := client.asOperator(http.MethodGet, orderPath, "")
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "received", field(t, recorder, "status"))
	})

	t.Run("gets own order as customer", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, client.do(http.MethodGet, orderPath, ownerToken, "").Code)
	})

	t.Run("forbids customer from reading another customer's order", func(t *testing.T) {
		otherPath := "/api/v1/service-orders/" + otherOrderID
		assert.Equal(t, http.StatusForbidden, client.do(http.MethodGet, otherPath, ownerToken, "").Code)
		assert.Equal(t, http.StatusForbidden, client.do(http.MethodGet, otherPath+"/history", ownerToken, "").Code)
		assert.Equal(t, http.StatusForbidden, client.do(http.MethodGet, otherPath+"/saga", ownerToken, "").Code)
	})

	t.Run("lists every active order to operator", func(t *testing.T) {
		recorder := client.asOperator(http.MethodGet, "/api/v1/service-orders", "")
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), orderID)
		assert.Contains(t, recorder.Body.String(), otherOrderID)
	})

	t.Run("lists only own orders to customer", func(t *testing.T) {
		recorder := client.do(http.MethodGet, "/api/v1/service-orders", ownerToken, "")
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), orderID)
		assert.NotContains(t, recorder.Body.String(), otherOrderID)
	})

	t.Run("exposes public status without token", func(t *testing.T) {
		recorder := client.do(http.MethodGet, orderPath+"/status", "", "")
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "received", field(t, recorder, "status"))
	})

	t.Run("returns history and saga", func(t *testing.T) {
		history := client.asOperator(http.MethodGet, orderPath+"/history", "")
		assert.Equal(t, http.StatusOK, history.Code)
		assert.Contains(t, history.Body.String(), "received")

		saga := client.do(http.MethodGet, orderPath+"/saga", ownerToken, "")
		assert.Equal(t, http.StatusOK, saga.Code)
		assert.Contains(t, saga.Body.String(), "RequestDiagnosis")
	})

	t.Run("rejects delivery before the order is finished", func(t *testing.T) {
		assert.Equal(t, http.StatusUnprocessableEntity, client.asOperator(http.MethodPatch, orderPath+"/deliver", "").Code)
	})

	t.Run("rejects cancellation without reason", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, client.asOperator(http.MethodPatch, orderPath+"/cancel", `{}`).Code)
	})

	t.Run("cancels order and starts compensation", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPatch, orderPath+"/cancel", `{"reason":"customer gave up"}`)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "cancelled", field(t, recorder, "status"))

		saga := client.asOperator(http.MethodGet, orderPath+"/saga", "")
		assert.Contains(t, saga.Body.String(), "compensating")
		assert.Contains(t, saga.Body.String(), "CancelWorkOrder")
	})

	t.Run("rejects cancelling twice", func(t *testing.T) {
		recorder := client.asOperator(http.MethodPatch, orderPath+"/cancel", `{"reason":"again"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	})

	t.Run("returns not found for unknown order", func(t *testing.T) {
		unknown := "/api/v1/service-orders/7d0a3c1b-2b1f-4f53-8a37-d1e6a7c49e55"
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodGet, unknown, "").Code)
		assert.Equal(t, http.StatusNotFound, client.do(http.MethodGet, unknown+"/status", "", "").Code)
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodGet, unknown+"/history", "").Code)
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodPatch, unknown+"/deliver", "").Code)
		assert.Equal(t, http.StatusNotFound, client.asOperator(http.MethodPatch, unknown+"/cancel", `{"reason":"x"}`).Code)
	})
}

func TestAPI_ShouldResolveCustomersForInternalCallers(t *testing.T) {
	client := newAPIClient(t)
	customerID := client.createCustomer("Maria Silva", "529.982.247-25", "maria@example.com")
	path := "/internal/customers/by-document/52998224725"

	t.Run("requires the internal secret", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, client.do(http.MethodGet, path, "", "").Code)
		assert.Equal(t, http.StatusUnauthorized, client.do(http.MethodGet, path, "", "", "X-Internal-Secret", "wrong").Code)
	})

	t.Run("returns customer identity", func(t *testing.T) {
		recorder := client.do(http.MethodGet, path, "", "", "X-Internal-Secret", apiInternalSecret)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, customerID, field(t, recorder, "id"))
		assert.Equal(t, "active", field(t, recorder, "status"))
	})

	t.Run("returns not found for unknown document", func(t *testing.T) {
		recorder := client.do(http.MethodGet, "/internal/customers/by-document/11144477735", "", "", "X-Internal-Secret", apiInternalSecret)
		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})
}
