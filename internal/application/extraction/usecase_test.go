package extraction_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appextraction "github.com/yoavweber/research-monitor/backend/internal/application/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	extractionrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
	"github.com/yoavweber/research-monitor/backend/tests/testdb"
)

// fixedClock is a deterministic shared.Clock for use case tests.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// fixture wires a fresh in-memory repo, a fake extractor, a recording logger,
// a frozen clock, and a buffered wake channel. Each subtest gets its own
// fixture so t.Parallel is safe.
type fixture struct {
	uc        extraction.UseCase
	repo      extraction.Repository
	extractor *mocks.ExtractorFake
	logger    *mocks.RecordingLogger
	clock     shared.Clock
	wakeCh    chan struct{}
}

func newFixture(t *testing.T, maxWords int) *fixture {
	t.Helper()
	db := testdb.New(t)
	repo := extractionrepo.NewRepository(db)
	extractor := &mocks.ExtractorFake{}
	logger := &mocks.RecordingLogger{}
	clock := fixedClock{t: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	wakeCh := make(chan struct{}, 2)

	uc := appextraction.NewExtractionUseCase(repo, extractor, logger, clock, wakeCh, maxWords)
	return &fixture{
		uc:        uc,
		repo:      repo,
		extractor: extractor,
		logger:    logger,
		clock:     clock,
		wakeCh:    wakeCh,
	}
}

func paperPayload(sourceID, pdfPath string) extraction.RequestPayload {
	return extraction.RequestPayload{
		SourceType: "paper",
		SourceID:   sourceID,
		PDFPath:    pdfPath,
	}
}

func TestSubmit(t *testing.T) {
	t.Parallel()

	t.Run("creates a new pending row and signals the wake channel", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx := context.Background()

		res, err := fx.uc.Submit(ctx, paperPayload("src-1", "/tmp/x.pdf"))

		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if res.ID == "" {
			t.Fatal("expected non-empty id")
		}
		if res.Status != extraction.JobStatusPending {
			t.Errorf("status = %q, want pending", res.Status)
		}
		got, err := fx.uc.Get(ctx, res.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Status != extraction.JobStatusPending {
			t.Errorf("Get.Status = %q, want pending", got.Status)
		}
		if got.RequestPayload.PDFPath != "/tmp/x.pdf" {
			t.Errorf("PDFPath = %q, want /tmp/x.pdf", got.RequestPayload.PDFPath)
		}
		if len(fx.wakeCh) != 1 {
			t.Errorf("len(wakeCh) = %d, want 1", len(fx.wakeCh))
		}
	})

	t.Run("rejects empty fields with ErrInvalidRequest", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx := context.Background()

		_, err := fx.uc.Submit(ctx, extraction.RequestPayload{SourceType: "paper", SourceID: "", PDFPath: "/tmp/x.pdf"})

		if !errors.Is(err, extraction.ErrInvalidRequest) {
			t.Fatalf("err = %v, want ErrInvalidRequest", err)
		}
		if len(fx.wakeCh) != 0 {
			t.Errorf("wake channel should not have been signalled on validation error")
		}
	})

	t.Run("rejects unsupported source_type with ErrUnsupportedSourceType", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx := context.Background()

		_, err := fx.uc.Submit(ctx, extraction.RequestPayload{SourceType: "html", SourceID: "src-1", PDFPath: "/tmp/x.pdf"})

		if !errors.Is(err, extraction.ErrUnsupportedSourceType) {
			t.Fatalf("err = %v, want ErrUnsupportedSourceType", err)
		}
		if len(fx.wakeCh) != 0 {
			t.Errorf("wake channel should not have been signalled on validation error")
		}
	})

	t.Run("overwrites a prior failed row, refreshes created_at, and emits one extraction.reextract log line", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx := context.Background()
		payload := paperPayload("src-overwrite", "/tmp/a.pdf")

		first, err := fx.uc.Submit(ctx, payload)
		if err != nil {
			t.Fatalf("first Submit: %v", err)
		}
		// Drain initial wake signal so we can assert one new signal afterwards.
		<-fx.wakeCh
		// Push the row through running -> failed so the prior state carries
		// a failure_reason worth logging.
		if err := fx.repo.ClaimPending(ctx, first.ID); err != nil {
			t.Fatalf("ClaimPending: %v", err)
		}
		if err := fx.repo.MarkFailed(ctx, first.ID, extraction.FailureReasonExtractorFailure, "boom"); err != nil {
			t.Fatalf("MarkFailed: %v", err)
		}
		firstRow, err := fx.uc.Get(ctx, first.ID)
		if err != nil {
			t.Fatalf("Get first: %v", err)
		}
		priorCreatedAt := firstRow.CreatedAt

		// Sleep for one millisecond so the SQLite-stored timestamp can move
		// forward; the underlying repo uses time.Now() directly.
		time.Sleep(2 * time.Millisecond)

		second, err := fx.uc.Submit(ctx, payload)

		if err != nil {
			t.Fatalf("second Submit: %v", err)
		}
		if second.ID != first.ID {
			t.Errorf("id changed across overwrite: %q -> %q", first.ID, second.ID)
		}
		if second.Status != extraction.JobStatusPending {
			t.Errorf("status = %q, want pending", second.Status)
		}
		secondRow, err := fx.uc.Get(ctx, second.ID)
		if err != nil {
			t.Fatalf("Get second: %v", err)
		}
		if !secondRow.CreatedAt.After(priorCreatedAt) {
			t.Errorf("created_at not refreshed: prior=%v new=%v", priorCreatedAt, secondRow.CreatedAt)
		}
		if len(fx.wakeCh) != 1 {
			t.Errorf("len(wakeCh) = %d, want 1 after overwrite Submit", len(fx.wakeCh))
		}
		infos := fx.logger.RecordsAt("Info")
		var reextract []mocks.LogRecord
		for _, r := range infos {
			if r.Msg == "extraction.reextract" {
				reextract = append(reextract, r)
			}
		}
		if len(reextract) != 1 {
			t.Fatalf("extraction.reextract count = %d, want 1", len(reextract))
		}
		args := reextract[0].Args
		if args["id"] != first.ID {
			t.Errorf("log id = %v, want %s", args["id"], first.ID)
		}
		if args["source_type"] != "paper" {
			t.Errorf("log source_type = %v, want paper", args["source_type"])
		}
		if args["source_id"] != "src-overwrite" {
			t.Errorf("log source_id = %v, want src-overwrite", args["source_id"])
		}
		if args["prior_status"] != string(extraction.JobStatusFailed) {
			t.Errorf("log prior_status = %v, want failed", args["prior_status"])
		}
		if args["prior_failure_reason"] != string(extraction.FailureReasonExtractorFailure) {
			t.Errorf("log prior_failure_reason = %v, want extractor_failure", args["prior_failure_reason"])
		}
	})
}

// claimRow is a test helper that submits a row and immediately transitions it
// to running so Process can be exercised against the worker-side invariant.
func claimRow(t *testing.T, fx *fixture, payload extraction.RequestPayload) extraction.Extraction {
	t.Helper()
	ctx := context.Background()
	res, err := fx.uc.Submit(ctx, payload)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := fx.repo.ClaimPending(ctx, res.ID); err != nil {
		t.Fatalf("ClaimPending: %v", err)
	}
	row, err := fx.uc.Get(ctx, res.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return *row
}

func TestProcess(t *testing.T) {
	t.Parallel()

	t.Run("happy path normalizes the body, strips references, and writes done with mirrored content_type", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx := context.Background()
		fx.extractor.Output = extraction.ExtractOutput{
			Markdown: strings.Join([]string{
				"# A Great Paper",
				"",
				"Body with inline math \\(x^2\\) and a citation.",
				"",
				"## References",
				"1. Ref one.",
				"2. Ref two.",
			}, "\n"),
		}
		row := claimRow(t, fx, paperPayload("src-happy", "/tmp/great.pdf"))

		if err := fx.uc.Process(ctx, row); err != nil {
			t.Fatalf("Process: %v", err)
		}

		got, err := fx.uc.Get(ctx, row.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Status != extraction.JobStatusDone {
			t.Fatalf("status = %q, want done", got.Status)
		}
		if got.Artifact == nil {
			t.Fatal("artifact is nil")
		}
		if got.Artifact.Title != "A Great Paper" {
			t.Errorf("title = %q, want %q", got.Artifact.Title, "A Great Paper")
		}
		if got.Artifact.Metadata.ContentType != "paper" {
			t.Errorf("content_type = %q, want paper", got.Artifact.Metadata.ContentType)
		}
		if got.Artifact.Metadata.WordCount <= 0 {
			t.Errorf("word_count = %d, want > 0", got.Artifact.Metadata.WordCount)
		}
		if strings.Contains(got.Artifact.BodyMarkdown, "Ref one") {
			t.Error("references tail was not stripped")
		}
		if !strings.Contains(got.Artifact.BodyMarkdown, "$x^2$") {
			t.Errorf("inline math not rewritten to $...$ form: %q", got.Artifact.BodyMarkdown)
		}
	})

	t.Run("uses the PDF basename without extension as fallback title when there is no level-1 heading", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx := context.Background()
		fx.extractor.Output = extraction.ExtractOutput{Markdown: "Some body without a heading"}
		row := claimRow(t, fx, paperPayload("src-fallback", "/tmp/foo bar.pdf"))

		if err := fx.uc.Process(ctx, row); err != nil {
			t.Fatalf("Process: %v", err)
		}

		got, err := fx.uc.Get(ctx, row.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Artifact == nil {
			t.Fatal("artifact is nil")
		}
		if got.Artifact.Title != "foo bar" {
			t.Errorf("title = %q, want %q", got.Artifact.Title, "foo bar")
		}
	})

	t.Run("maps ErrScannedPDF to failed: scanned_pdf", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx := context.Background()
		fx.extractor.Err = extraction.ErrScannedPDF
		row := claimRow(t, fx, paperPayload("src-scanned", "/tmp/scan.pdf"))

		if err := fx.uc.Process(ctx, row); err != nil {
			t.Fatalf("Process: %v", err)
		}

		got, err := fx.uc.Get(ctx, row.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Status != extraction.JobStatusFailed {
			t.Fatalf("status = %q, want failed", got.Status)
		}
		if got.Failure == nil || got.Failure.Reason != extraction.FailureReasonScannedPDF {
			t.Errorf("failure = %+v, want reason=scanned_pdf", got.Failure)
		}
	})

	t.Run("maps ErrParseFailed to failed: parse_failed", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx := context.Background()
		fx.extractor.Err = extraction.ErrParseFailed
		row := claimRow(t, fx, paperPayload("src-parse", "/tmp/bad.pdf"))

		if err := fx.uc.Process(ctx, row); err != nil {
			t.Fatalf("Process: %v", err)
		}

		got, _ := fx.uc.Get(ctx, row.ID)
		if got.Status != extraction.JobStatusFailed {
			t.Fatalf("status = %q, want failed", got.Status)
		}
		if got.Failure == nil || got.Failure.Reason != extraction.FailureReasonParseFailed {
			t.Errorf("failure = %+v, want reason=parse_failed", got.Failure)
		}
	})

	t.Run("maps ErrExtractorFailure to failed: extractor_failure", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx := context.Background()
		fx.extractor.Err = extraction.ErrExtractorFailure
		row := claimRow(t, fx, paperPayload("src-failure", "/tmp/x.pdf"))

		if err := fx.uc.Process(ctx, row); err != nil {
			t.Fatalf("Process: %v", err)
		}

		got, _ := fx.uc.Get(ctx, row.ID)
		if got.Status != extraction.JobStatusFailed {
			t.Fatalf("status = %q, want failed", got.Status)
		}
		if got.Failure == nil || got.Failure.Reason != extraction.FailureReasonExtractorFailure {
			t.Errorf("failure = %+v, want reason=extractor_failure", got.Failure)
		}
	})

	t.Run("flips word_count above max_words to failed: too_large with both numbers in the message", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 5)
		ctx := context.Background()
		fx.extractor.Output = extraction.ExtractOutput{
			Markdown: "one two three four five six seven eight nine ten",
		}
		row := claimRow(t, fx, paperPayload("src-large", "/tmp/big.pdf"))

		if err := fx.uc.Process(ctx, row); err != nil {
			t.Fatalf("Process: %v", err)
		}

		got, _ := fx.uc.Get(ctx, row.ID)
		if got.Status != extraction.JobStatusFailed {
			t.Fatalf("status = %q, want failed", got.Status)
		}
		if got.Failure == nil || got.Failure.Reason != extraction.FailureReasonTooLarge {
			t.Fatalf("failure = %+v, want reason=too_large", got.Failure)
		}
		if !strings.Contains(got.Failure.Message, "10") || !strings.Contains(got.Failure.Message, "5") {
			t.Errorf("failure message = %q, want both 10 and 5", got.Failure.Message)
		}
	})

	t.Run("on ctx cancellation leaves the row in running and writes nothing", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx, cancel := context.WithCancel(context.Background())
		fx.extractor.Err = context.Canceled
		row := claimRow(t, fx, paperPayload("src-cancel", "/tmp/x.pdf"))
		cancel()

		err := fx.uc.Process(ctx, row)

		if err == nil {
			t.Fatal("Process err = nil, want ctx error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		got, getErr := fx.uc.Get(context.Background(), row.ID)
		if getErr != nil {
			t.Fatalf("Get: %v", getErr)
		}
		if got.Status != extraction.JobStatusRunning {
			t.Errorf("status = %q, want running", got.Status)
		}
	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	t.Run("returns ErrNotFound for an unknown id", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t, 100_000)
		ctx := context.Background()

		_, err := fx.uc.Get(ctx, "no-such-id")

		if !errors.Is(err, extraction.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}
