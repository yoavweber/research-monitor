package mocks

import (
	"context"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// ExtractionRepoFindByIDOnly satisfies extraction.Repository for callers
// that only legitimately read by id (the analyzer use case is the canonical
// example). Every other method panics so an unintended call surfaces loudly
// instead of silently no-oping.
type ExtractionRepoFindByIDOnly struct {
	Row *extraction.Extraction
	Err error
}

func (s *ExtractionRepoFindByIDOnly) FindByID(_ context.Context, _ string) (*extraction.Extraction, error) {
	return s.Row, s.Err
}

func (s *ExtractionRepoFindByIDOnly) Upsert(context.Context, extraction.RequestPayload) (string, *extraction.PriorState, error) {
	panic("Upsert must not be called from a read-only consumer")
}
func (s *ExtractionRepoFindByIDOnly) ClaimPending(context.Context, string) error {
	panic("ClaimPending must not be called from a read-only consumer")
}
func (s *ExtractionRepoFindByIDOnly) MarkDone(context.Context, string, extraction.Artifact) error {
	panic("MarkDone must not be called from a read-only consumer")
}
func (s *ExtractionRepoFindByIDOnly) MarkFailed(context.Context, string, extraction.FailureReason, string) error {
	panic("MarkFailed must not be called from a read-only consumer")
}
func (s *ExtractionRepoFindByIDOnly) PeekNextPending(context.Context) (*extraction.Extraction, bool, error) {
	panic("PeekNextPending must not be called from a read-only consumer")
}
func (s *ExtractionRepoFindByIDOnly) RecoverRunningOnStartup(context.Context) (int, error) {
	panic("RecoverRunningOnStartup must not be called from a read-only consumer")
}
func (s *ExtractionRepoFindByIDOnly) ListPendingIDs(context.Context) ([]string, error) {
	panic("ListPendingIDs must not be called from a read-only consumer")
}
