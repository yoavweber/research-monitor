package controller

import (
	domain "github.com/yoavweber/research-monitor/backend/internal/domain/source"
)

// SourceEnvelope and SourceListEnvelope are schema-only wrappers used by the
// @Success annotations on the /api/sources endpoints. They exist so the
// OpenAPI schema accurately describes the {"data": ...} runtime envelope;
// neither is ever instantiated.
type SourceEnvelope struct {
	Data domain.Response `json:"data"`
}

type SourceListEnvelope struct {
	Data []domain.Response `json:"data"`
}
