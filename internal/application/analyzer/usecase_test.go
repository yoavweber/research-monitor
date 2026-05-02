package analyzer_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	app "github.com/yoavweber/research-monitor/backend/internal/application/analyzer"
	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/llm/stub"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

// inMemoryRepo is a shared.Repository fake scoped to this test file. It is
// deliberately tiny: the persistence-layer tests cover the upsert contract
// against a real DB; here we only need to observe whether Upsert was called.
type inMemoryRepo struct {
	mu       sync.Mutex
	rows     map[string]domain.Analysis
	upsertEr error
	findErr  error
	upserts  int
}

func newInMemoryRepo() *inMemoryRepo {
	return &inMemoryRepo{rows: map[string]domain.Analysis{}}
}

func (r *inMemoryRepo) Upsert(_ context.Context, a domain.Analysis) (domain.Analysis, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserts++
	if r.upsertEr != nil {
		return domain.Analysis{}, r.upsertEr
	}
	if prior, ok := r.rows[a.ExtractionID]; ok {
		a.CreatedAt = prior.CreatedAt
	}
	r.rows[a.ExtractionID] = a
	return a, nil
}

func (r *inMemoryRepo) FindByID(_ context.Context, id string) (*domain.Analysis, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.findErr != nil {
		return nil, r.findErr
	}
	row, ok := r.rows[id]
	if !ok {
		return nil, domain.ErrAnalysisNotFound
	}
	return &row, nil
}

// stubExtractionRepo is the minimum extraction.Repository surface the use
// case touches: only FindByID. Other methods panic if called — failure to
// stay within the read-only contract should be loud.
type stubExtractionRepo struct {
	row *extraction.Extraction
	err error
}

func (s *stubExtractionRepo) FindByID(_ context.Context, _ string) (*extraction.Extraction, error) {
	return s.row, s.err
}

func (s *stubExtractionRepo) Upsert(context.Context, extraction.RequestPayload) (string, *extraction.PriorState, error) {
	panic("Upsert must not be called from the analyzer use case")
}
func (s *stubExtractionRepo) ClaimPending(context.Context, string) error {
	panic("ClaimPending must not be called from the analyzer use case")
}
func (s *stubExtractionRepo) MarkDone(context.Context, string, extraction.Artifact) error {
	panic("MarkDone must not be called from the analyzer use case")
}
func (s *stubExtractionRepo) MarkFailed(context.Context, string, extraction.FailureReason, string) error {
	panic("MarkFailed must not be called from the analyzer use case")
}
func (s *stubExtractionRepo) PeekNextPending(context.Context) (*extraction.Extraction, bool, error) {
	panic("PeekNextPending must not be called from the analyzer use case")
}
func (s *stubExtractionRepo) RecoverRunningOnStartup(context.Context) (int, error) {
	panic("RecoverRunningOnStartup must not be called from the analyzer use case")
}
func (s *stubExtractionRepo) ListPendingIDs(context.Context) ([]string, error) {
	panic("ListPendingIDs must not be called from the analyzer use case")
}

func doneExtraction(id, body string) *extraction.Extraction {
	return &extraction.Extraction{
		ID:     id,
		Status: extraction.JobStatusDone,
		Artifact: &extraction.Artifact{
			BodyMarkdown: body,
			Title:        "Test paper",
		},
	}
}

type harness struct {
	uc      domain.UseCase
	repo    *inMemoryRepo
	llm     *stub.Client
	clock   *mocks.MovableClock
	extract *stubExtractionRepo
}

func newHarness(extract *stubExtractionRepo) *harness {
	repo := newInMemoryRepo()
	llm := stub.New()
	clock := mocks.NewMovableClock(time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC))
	logger := &mocks.RecordingLogger{}
	uc := app.NewAnalyzerUseCase(repo, extract, llm, logger, clock)
	return &harness{uc: uc, repo: repo, llm: llm, clock: clock, extract: extract}
}

func (h *harness) queueValidThree() {
	h.llm.QueueResponse("short summary text")
	h.llm.QueueResponse("long summary text body")
	h.llm.Results = append(h.llm.Results, stub.Result{
		Response: &shared.LLMResponse{
			Text:  `{"flag": true, "rationale": "promising thesis angle"}`,
			Model: "fake-thesis-model",
		},
	})
}

// --- success path ---------------------------------------------------------

func TestAnalyze_HappyPath_PersistsAndReturnsAnalysis(t *testing.T) {
	t.Parallel()

	h := newHarness(&stubExtractionRepo{row: doneExtraction("ex-1", "body markdown")})
	h.queueValidThree()

	got, err := h.uc.Analyze(context.Background(), "ex-1")

	if err != nil {
		t.Fatalf("Analyze err = %v, want nil", err)
	}
	if got == nil {
		t.Fatal("Analyze returned nil analysis")
	}
	if got.ExtractionID != "ex-1" {
		t.Errorf("ExtractionID = %q, want %q", got.ExtractionID, "ex-1")
	}
	if got.ShortSummary != "short summary text" {
		t.Errorf("ShortSummary = %q", got.ShortSummary)
	}
	if got.LongSummary != "long summary text body" {
		t.Errorf("LongSummary = %q", got.LongSummary)
	}
	if !got.ThesisAngleFlag {
		t.Errorf("ThesisAngleFlag = false, want true")
	}
	if got.ThesisAngleRationale != "promising thesis angle" {
		t.Errorf("ThesisAngleRationale = %q", got.ThesisAngleRationale)
	}
	if got.Model != "fake-thesis-model" {
		t.Errorf("Model = %q, want the thesis call's response model", got.Model)
	}
	if got.PromptVersion != app.PromptVersionComposite {
		t.Errorf("PromptVersion = %q, want %q", got.PromptVersion, app.PromptVersionComposite)
	}
	if h.repo.upserts != 1 {
		t.Errorf("upsert calls = %d, want 1", h.repo.upserts)
	}
	if h.llm.CallCount != 3 {
		t.Errorf("LLM call count = %d, want 3", h.llm.CallCount)
	}
	versions := []string{h.llm.Calls[0].PromptVersion, h.llm.Calls[1].PromptVersion, h.llm.Calls[2].PromptVersion}
	want := []string{app.PromptVersionShort, app.PromptVersionLong, app.PromptVersionThesis}
	for i := range want {
		if versions[i] != want[i] {
			t.Errorf("call %d PromptVersion = %q, want %q", i, versions[i], want[i])
		}
	}
}

// --- precondition failures ------------------------------------------------

func TestAnalyze_ExtractionNotFound_ReturnsSentinelAndSkipsLLM(t *testing.T) {
	t.Parallel()

	h := newHarness(&stubExtractionRepo{err: extraction.ErrNotFound})

	_, err := h.uc.Analyze(context.Background(), "missing")

	if !errors.Is(err, domain.ErrExtractionNotFound) {
		t.Fatalf("err = %v, want ErrExtractionNotFound", err)
	}
	if h.llm.CallCount != 0 {
		t.Errorf("LLM call count = %d, want 0 on precondition failure", h.llm.CallCount)
	}
	if h.repo.upserts != 0 {
		t.Errorf("upserts = %d, want 0 on precondition failure", h.repo.upserts)
	}
}

func TestAnalyze_ExtractionNotDone_ReturnsNotReadyAndSkipsLLM(t *testing.T) {
	t.Parallel()

	pending := &extraction.Extraction{ID: "ex-pending", Status: extraction.JobStatusPending}
	h := newHarness(&stubExtractionRepo{row: pending})

	_, err := h.uc.Analyze(context.Background(), "ex-pending")

	if !errors.Is(err, domain.ErrExtractionNotReady) {
		t.Fatalf("err = %v, want ErrExtractionNotReady", err)
	}
	if h.llm.CallCount != 0 {
		t.Errorf("LLM call count = %d, want 0 on precondition failure", h.llm.CallCount)
	}
	if h.repo.upserts != 0 {
		t.Errorf("upserts = %d, want 0 on precondition failure", h.repo.upserts)
	}
}

// --- LLM transport failures -----------------------------------------------

func TestAnalyze_LLMTransportError_OnEachCall_FailsFastNoRow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		failOn   int // 0-indexed call to fail
		expected int // calls completed before failure (= failOn)
	}{
		{"short call fails", 0, 1},
		{"long call fails", 1, 2},
		{"thesis call fails", 2, 3},
	}

	for _, tc := range cases {
		tc := tc
		t.Run("returns ErrLLMUpstream when the "+tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(&stubExtractionRepo{row: doneExtraction("ex-fail", "body")})
			for i := 0; i < tc.failOn; i++ {
				h.llm.QueueResponse("ok")
			}
			h.llm.QueueError(errors.New("transport boom"))

			_, err := h.uc.Analyze(context.Background(), "ex-fail")

			if !errors.Is(err, domain.ErrLLMUpstream) {
				t.Fatalf("err = %v, want ErrLLMUpstream", err)
			}
			if h.llm.CallCount != tc.expected {
				t.Errorf("LLM call count = %d, want %d (no retry)", h.llm.CallCount, tc.expected)
			}
			if h.repo.upserts != 0 {
				t.Errorf("upserts = %d, want 0 on transport failure", h.repo.upserts)
			}
		})
	}
}

// --- malformed thesis envelope --------------------------------------------

func TestAnalyze_ThesisEnvelopeMalformed_FailsFastNoRow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		thesisReply string
	}{
		{"unparseable JSON", "definitely not json"},
		{"missing flag", `{"rationale": "no flag here"}`},
		{"missing rationale", `{"flag": true}`},
		{"non-boolean flag", `{"flag": "yes", "rationale": "bad type"}`},
		{"non-string rationale", `{"flag": true, "rationale": 42}`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run("returns ErrAnalyzerMalformedResponse for "+tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(&stubExtractionRepo{row: doneExtraction("ex-bad", "body")})
			h.llm.QueueResponse("short")
			h.llm.QueueResponse("long")
			h.llm.QueueResponse(tc.thesisReply)

			_, err := h.uc.Analyze(context.Background(), "ex-bad")

			if !errors.Is(err, domain.ErrAnalyzerMalformedResponse) {
				t.Fatalf("err = %v, want ErrAnalyzerMalformedResponse", err)
			}
			if h.repo.upserts != 0 {
				t.Errorf("upserts = %d, want 0 on malformed envelope", h.repo.upserts)
			}
			if h.llm.CallCount != 3 {
				t.Errorf("LLM call count = %d, want 3 (no retry on parse failure)", h.llm.CallCount)
			}
		})
	}
}

// --- Get -------------------------------------------------------------------

func TestGet_NeverInvokesLLM(t *testing.T) {
	t.Parallel()

	h := newHarness(&stubExtractionRepo{}) // unused
	h.repo.rows["ex-1"] = domain.Analysis{ExtractionID: "ex-1", ShortSummary: "saved"}

	got, err := h.uc.Get(context.Background(), "ex-1")

	if err != nil {
		t.Fatalf("Get err = %v, want nil", err)
	}
	if got == nil || got.ShortSummary != "saved" {
		t.Errorf("Get returned %+v, want the persisted row", got)
	}
	if h.llm.CallCount != 0 {
		t.Errorf("Get invoked LLM %d time(s), want 0", h.llm.CallCount)
	}
}

func TestGet_ReturnsAnalysisNotFoundFromRepo(t *testing.T) {
	t.Parallel()

	h := newHarness(&stubExtractionRepo{})

	_, err := h.uc.Get(context.Background(), "missing")

	if !errors.Is(err, domain.ErrAnalysisNotFound) {
		t.Fatalf("err = %v, want ErrAnalysisNotFound", err)
	}
}
