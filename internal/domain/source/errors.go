package source

import (
	"net/http"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
)

var (
	ErrNotFound = shared.NewHTTPError(http.StatusNotFound, "source not found", nil)
	ErrConflict = shared.NewHTTPError(http.StatusConflict, "source url already exists", nil)
)
