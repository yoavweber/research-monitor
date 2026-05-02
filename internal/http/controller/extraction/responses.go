package extraction

import (
	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// ExtractionStatusResponse is the wire shape returned by both POST and GET.
// Artifact fields render only on status=done; failure fields only on
// status=failed; otherwise both groups are omitted via omitempty so
// pending/running responses carry only the identity + status quartet.
type ExtractionStatusResponse struct {
	ID         string `json:"id"`
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	Status     string `json:"status"`

	// Artifact branch — populated iff status == done. Metadata is a pointer
	// so the entire object disappears from JSON when nil.
	Title        string       `json:"title,omitempty"`
	BodyMarkdown string       `json:"body_markdown,omitempty"`
	Metadata     *MetadataDTO `json:"metadata,omitempty"`

	// Failure branch — populated iff status == failed.
	FailureReason  string `json:"failure_reason,omitempty"`
	FailureMessage string `json:"failure_message,omitempty"`
}

// MetadataDTO is the wire shape for Artifact.Metadata. Pointer use in the
// parent (ExtractionStatusResponse.Metadata) is the mechanism that drops
// the whole metadata block from JSON when status != done.
type MetadataDTO struct {
	ContentType string `json:"content_type"`
	WordCount   int    `json:"word_count"`
}

// ExtractionStatusEnvelope is the @Success-annotation wrapper used by swag.
// It mirrors the runtime envelope produced by common.Data so the generated
// OpenAPI doc carries the right top-level "data" wrapping.
type ExtractionStatusEnvelope struct {
	Data ExtractionStatusResponse `json:"data"`
}

// ToExtractionStatusResponse maps a domain.Extraction to the wire DTO,
// applying the conditional-rendering rule on status: artifact fields render
// iff status == done and Artifact != nil; failure fields render iff status
// == failed and Failure != nil; pending / running responses leave both
// groups zero-valued so omitempty drops them.
func ToExtractionStatusResponse(e extraction.Extraction) ExtractionStatusResponse {
	resp := ExtractionStatusResponse{
		ID:         e.ID,
		SourceType: e.SourceType,
		SourceID:   e.SourceID,
		Status:     string(e.Status),
	}
	if e.Status == extraction.JobStatusDone && e.Artifact != nil {
		resp.Title = e.Artifact.Title
		resp.BodyMarkdown = e.Artifact.BodyMarkdown
		resp.Metadata = &MetadataDTO{
			ContentType: e.Artifact.Metadata.ContentType,
			WordCount:   e.Artifact.Metadata.WordCount,
		}
	}
	if e.Status == extraction.JobStatusFailed && e.Failure != nil {
		resp.FailureReason = string(e.Failure.Reason)
		resp.FailureMessage = e.Failure.Message
	}
	return resp
}
