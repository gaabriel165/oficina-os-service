package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielcamargo/oficina-api/internal/api/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func performInternalRequest(t *testing.T, secret, header string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	router := gin.New()
	router.Use(middleware.InternalAuth(secret))
	router.GET("/internal", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodGet, "/internal", nil)
	if header != "" {
		request.Header.Set(middleware.InternalSecretHeader, header)
	}
	router.ServeHTTP(recorder, request)

	return recorder
}

func TestInternalAuth_ShouldAllowWhenSecretMatches(t *testing.T) {
	assert.Equal(t, http.StatusOK, performInternalRequest(t, "top-secret", "top-secret").Code)
}

func TestInternalAuth_ShouldRejectWhenSecretMismatches(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, performInternalRequest(t, "top-secret", "wrong").Code)
}

func TestInternalAuth_ShouldRejectWhenHeaderMissing(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, performInternalRequest(t, "top-secret", "").Code)
}

func TestInternalAuth_ShouldRejectWhenSecretNotConfigured(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, performInternalRequest(t, "", "anything").Code)
}
