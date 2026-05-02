// Package extraction hosts the HTTP handler and wire DTOs for the
// POST /api/extractions and GET /api/extractions/:id endpoints. The domain
// layer (internal/domain/extraction) carries no transport shapes — this
// package owns the JSON contract and the mapping from extraction.Extraction
// into it.
package extraction

// SubmitExtractionRequest is the inbound DTO bound from POST /api/extractions.
// `binding:"required"` rejects empty values via Gin's binding before the
// domain SubmitRequest.Validate runs. The controller translates this thin
// wire shape into the domain RequestPayload so the use case re-validates
// through the same path non-HTTP entrypoints use.
type SubmitExtractionRequest struct {
	SourceType string `json:"source_type" binding:"required"`
	SourceID   string `json:"source_id" binding:"required"`
	PDFPath    string `json:"pdf_path" binding:"required"`
}
