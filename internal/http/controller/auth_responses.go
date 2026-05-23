package controller

import (
	"github.com/yoavweber/research-monitor/backend/internal/domain/user"
)

// LoginEnvelope and SessionEnvelope are schema-only wrappers used by the
// @Success annotations on the /auth endpoints. They exist so the generated
// OpenAPI schema accurately describes the {"data": ...} runtime envelope
// produced by common.Data; neither is ever instantiated at runtime.
type LoginEnvelope struct {
	Data user.LoginResponse `json:"data"`
}

type SessionEnvelope struct {
	Data user.SessionResponse `json:"data"`
}
