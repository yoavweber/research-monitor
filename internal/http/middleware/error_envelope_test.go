package middleware_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func errorEnvelopeServer(handlerErr error) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ErrorEnvelope())
	r.GET("/boom", func(c *gin.Context) {
		_ = c.Error(handlerErr)
	})
	return r
}

func decodeEnvelope(t *testing.T, body []byte) common.Envelope {
	t.Helper()
	var env common.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, body)
	}
	return env
}

func TestErrorEnvelope(t *testing.T) {
	t.Parallel()

	t.Run("renders an HTTPError without Reason as the existing envelope shape", func(t *testing.T) {
		t.Parallel()

		he := shared.NewHTTPError(http.StatusNotFound, "thing not found", nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/boom", nil)

		errorEnvelopeServer(he).ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
		env := decodeEnvelope(t, w.Body.Bytes())
		if env.Error == nil {
			t.Fatal("envelope.Error is nil")
		}
		if env.Error.Code != http.StatusNotFound || env.Error.Message != "thing not found" {
			t.Fatalf("envelope.Error = %+v, want code=404 message=%q", env.Error, "thing not found")
		}
		if _, present := env.Error.Details["reason"]; present {
			t.Fatalf("envelope.Error.Details.reason must be absent when Reason is empty; got %v", env.Error.Details)
		}
	})

	t.Run("surfaces a non-empty Reason under error.details.reason", func(t *testing.T) {
		t.Parallel()

		he := shared.NewHTTPError(http.StatusBadGateway, "llm upstream failed", nil).WithReason("llm_upstream")
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/boom", nil)

		errorEnvelopeServer(he).ServeHTTP(w, req)

		if w.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
		}
		env := decodeEnvelope(t, w.Body.Bytes())
		if env.Error == nil {
			t.Fatal("envelope.Error is nil")
		}
		gotReason, ok := env.Error.Details["reason"].(string)
		if !ok {
			t.Fatalf("envelope.Error.Details.reason missing or not a string: %v", env.Error.Details)
		}
		if gotReason != "llm_upstream" {
			t.Fatalf("envelope.Error.Details.reason = %q, want %q", gotReason, "llm_upstream")
		}
	})

	t.Run("maps a non-HTTPError to a 500 envelope without details", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/boom", nil)

		errorEnvelopeServer(errors.New("some plain error")).ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
		env := decodeEnvelope(t, w.Body.Bytes())
		if env.Error == nil || env.Error.Code != http.StatusInternalServerError {
			t.Fatalf("envelope.Error = %+v, want code=500", env.Error)
		}
		if _, present := env.Error.Details["reason"]; present {
			t.Fatalf("envelope.Error.Details.reason must be absent for plain errors; got %v", env.Error.Details)
		}
	})
}
