package analyzer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/yoavweber/research-monitor/backend/internal/application/analyzer"
	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/llm/stub"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

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
	repo    *mocks.InMemoryAnalyzerRepo
	llm     *stub.Client
	clock   *mocks.MovableClock
	extract *mocks.ExtractionRepoFindByIDOnly
}

func newHarness(extract *mocks.ExtractionRepoFindByIDOnly) *harness {
	repo := mocks.NewInMemoryAnalyzerRepo()
	llm := stub.New()
	clock := mocks.NewMovableClock(time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC))
	logger := &mocks.RecordingLogger{}
	uc := app.NewAnalyzerUseCase(repo, extract, llm, logger, clock)
	return &harness{uc: uc, repo: repo, llm: llm, clock: clock, extract: extract}
}

func TestAnalyze(t *testing.T) {
	t.Parallel()

	t.Run("happy path persists short and long with placeholder thesis values", func(t *testing.T) {
		t.Parallel()

		h := newHarness(&mocks.ExtractionRepoFindByIDOnly{Row: doneExtraction("ex-1", "body markdown")})
		h.llm.QueueResponse("short summary text")
		h.llm.QueueResponse("long summary text body")

		got, err := h.uc.Analyze(context.Background(), "ex-1")

		if err != nil {
			t.Fatalf("Analyze err = %v, want nil", err)
		}
		if got.ExtractionID != "ex-1" {
			t.Errorf("ExtractionID = %q", got.ExtractionID)
		}
		if got.ShortSummary != "short summary text" {
			t.Errorf("ShortSummary = %q", got.ShortSummary)
		}
		if got.LongSummary != "long summary text body" {
			t.Errorf("LongSummary = %q", got.LongSummary)
		}
		if !got.ThesisAngleFlag {
			t.Errorf("ThesisAngleFlag = false; placeholder must be true until the thesis classifier ships")
		}
		if got.ThesisAngleRationale == "" {
			t.Errorf("ThesisAngleRationale empty; placeholder rationale must be present")
		}
		if got.PromptVersion != app.PromptVersionComposite {
			t.Errorf("PromptVersion = %q, want %q", got.PromptVersion, app.PromptVersionComposite)
		}
		if h.repo.Upserts != 1 {
			t.Errorf("upsert calls = %d, want 1", h.repo.Upserts)
		}
		if h.llm.CallCount != 2 {
			t.Errorf("LLM call count = %d, want 2", h.llm.CallCount)
		}
		want := []string{app.PromptVersionShort, app.PromptVersionLong}
		for i, w := range want {
			if h.llm.Calls[i].PromptVersion != w {
				t.Errorf("call %d PromptVersion = %q, want %q", i, h.llm.Calls[i].PromptVersion, w)
			}
		}
	})

	t.Run("returns ErrExtractionNotFound and skips LLM when extraction is missing", func(t *testing.T) {
		t.Parallel()

		h := newHarness(&mocks.ExtractionRepoFindByIDOnly{Err: extraction.ErrNotFound})

		_, err := h.uc.Analyze(context.Background(), "missing")

		if !errors.Is(err, domain.ErrExtractionNotFound) {
			t.Fatalf("err = %v, want ErrExtractionNotFound", err)
		}
		if h.llm.CallCount != 0 {
			t.Errorf("LLM call count = %d, want 0 on precondition failure", h.llm.CallCount)
		}
		if h.repo.Upserts != 0 {
			t.Errorf("upserts = %d, want 0 on precondition failure", h.repo.Upserts)
		}
	})

	t.Run("returns ErrExtractionNotReady and skips LLM when extraction is not done", func(t *testing.T) {
		t.Parallel()

		pending := &extraction.Extraction{ID: "ex-pending", Status: extraction.JobStatusPending}
		h := newHarness(&mocks.ExtractionRepoFindByIDOnly{Row: pending})

		_, err := h.uc.Analyze(context.Background(), "ex-pending")

		if !errors.Is(err, domain.ErrExtractionNotReady) {
			t.Fatalf("err = %v, want ErrExtractionNotReady", err)
		}
		if h.llm.CallCount != 0 {
			t.Errorf("LLM call count = %d, want 0 on precondition failure", h.llm.CallCount)
		}
		if h.repo.Upserts != 0 {
			t.Errorf("upserts = %d, want 0 on precondition failure", h.repo.Upserts)
		}
	})

	t.Run("returns ErrLLMUpstream and writes no row when transport fails", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name           string
			failOnCall     int
			expectedCalls  int
			preFailScripts int
		}{
			{"short call fails", 0, 1, 0},
			{"long call fails", 1, 2, 1},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				h := newHarness(&mocks.ExtractionRepoFindByIDOnly{Row: doneExtraction("ex-fail", "body")})
				for i := 0; i < tc.preFailScripts; i++ {
					h.llm.QueueResponse("ok")
				}
				h.llm.QueueError(errors.New("transport boom"))

				_, err := h.uc.Analyze(context.Background(), "ex-fail")

				if !errors.Is(err, domain.ErrLLMUpstream) {
					t.Fatalf("err = %v, want ErrLLMUpstream", err)
				}
				if h.llm.CallCount != tc.expectedCalls {
					t.Errorf("LLM call count = %d, want %d (no retry)", h.llm.CallCount, tc.expectedCalls)
				}
				if h.repo.Upserts != 0 {
					t.Errorf("upserts = %d, want 0 on transport failure", h.repo.Upserts)
				}
			})
		}
	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	t.Run("returns the persisted analysis and never invokes the LLM", func(t *testing.T) {
		t.Parallel()

		h := newHarness(&mocks.ExtractionRepoFindByIDOnly{})
		h.repo.Rows["ex-1"] = domain.Analysis{ExtractionID: "ex-1", ShortSummary: "saved"}

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
	})

	t.Run("returns ErrAnalysisNotFound for an unknown id", func(t *testing.T) {
		t.Parallel()

		h := newHarness(&mocks.ExtractionRepoFindByIDOnly{})

		_, err := h.uc.Get(context.Background(), "missing")

		if !errors.Is(err, domain.ErrAnalysisNotFound) {
			t.Fatalf("err = %v, want ErrAnalysisNotFound", err)
		}
	})
}
