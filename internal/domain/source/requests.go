package source

import (
	"net/http"
	"strings"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

type CreateRequest struct {
	Name string `json:"name" binding:"required"`
	Kind Kind   `json:"kind" binding:"required"`
	URL  string `json:"url"  binding:"required"`
}

func (r CreateRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return shared.NewHTTPError(http.StatusBadRequest, "name is required", nil)
	}
	if !r.Kind.Valid() {
		return shared.NewHTTPError(http.StatusBadRequest, "kind must be 'rss' or 'api'", nil)
	}
	if !strings.HasPrefix(r.URL, "http://") && !strings.HasPrefix(r.URL, "https://") {
		return shared.NewHTTPError(http.StatusBadRequest, "url must start with http:// or https://", nil)
	}
	return nil
}

type UpdateRequest struct {
	Name     *string `json:"name,omitempty"`
	IsActive *bool   `json:"is_active,omitempty"`
}

func (r UpdateRequest) Validate() error {
	if r.Name != nil && strings.TrimSpace(*r.Name) == "" {
		return shared.NewHTTPError(http.StatusBadRequest, "name cannot be empty", nil)
	}
	return nil
}
