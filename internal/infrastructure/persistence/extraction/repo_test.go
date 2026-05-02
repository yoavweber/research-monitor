package extraction_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	extractionpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	"github.com/yoavweber/research-monitor/backend/tests/testdb"
)

// newPayload returns a populated RequestPayload pinned to a stable
// (SourceType, SourceID) so dedupe tests don't drift between subtests.
func newPayload(sourceID, pdfPath string) domain.RequestPayload {
	return domain.RequestPayload{
		SourceType: "paper",
		SourceID:   sourceID,
		PDFPath:    pdfPath,
	}
}

func TestRepository_Upsert_InsertsPendingRow(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	id, prior, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/papers/2404.12345.pdf"))

	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if prior != nil {
		t.Fatalf("prior = %+v, want nil on first insert", prior)
	}
	if _, parseErr := uuid.Parse(id); parseErr != nil {
		t.Fatalf("id = %q is not a valid UUID: %v", id, parseErr)
	}

	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Status != domain.JobStatusPending {
		t.Errorf("status = %q, want pending", got.Status)
	}
	if got.RequestPayload != newPayload("2404.12345", "/tmp/papers/2404.12345.pdf") {
		t.Errorf("payload = %+v, want roundtrip match", got.RequestPayload)
	}
}

func TestRepository_Upsert_OverwritesAndReturnsPriorState(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	repo := extractionpersist.NewRepository(db)
	ctx := context.Background()

	originalID, _, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/papers/2404.12345.pdf"))
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Drive the row to running -> done so we can prove prior.Status surfaces.
	if err := repo.ClaimPending(ctx, originalID); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := repo.MarkDone(ctx, originalID, domain.Artifact{
		Title:        "Sample",
		BodyMarkdown: "body",
		Metadata:     domain.Metadata{ContentType: "paper", WordCount: 1},
	}); err != nil {
		t.Fatalf("mark done: %v", err)
	}

	priorRow, err := repo.FindByID(ctx, originalID)
	if err != nil {
		t.Fatalf("find prior: %v", err)
	}
	preCreatedAt := priorRow.CreatedAt

	// Sleep enough that any clock granularity surfaces a strictly-later created_at.
	time.Sleep(10 * time.Millisecond)

	gotID, prior, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/papers/2404.12345-v2.pdf"))
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if gotID != originalID {
		t.Errorf("id = %q, want preserved %q on overwrite", gotID, originalID)
	}
	if prior == nil {
		t.Fatalf("prior = nil, want non-nil PriorState on overwrite")
	}
	if prior.Status != domain.JobStatusDone {
		t.Errorf("prior.Status = %q, want done", prior.Status)
	}

	got, err := repo.FindByID(ctx, originalID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Status != domain.JobStatusPending {
		t.Errorf("status = %q, want reset to pending", got.Status)
	}
	if got.Artifact != nil {
		t.Errorf("artifact = %+v, want cleared", got.Artifact)
	}
	if got.RequestPayload.PDFPath != "/tmp/papers/2404.12345-v2.pdf" {
		t.Errorf("PDFPath = %q, want overwrite to land", got.RequestPayload.PDFPath)
	}
	if !got.CreatedAt.After(preCreatedAt) {
		t.Errorf("created_at = %v, want strictly after prior %v", got.CreatedAt, preCreatedAt)
	}
}

func TestRepository_Upsert_OverwriteFromFailedScannedPDFPreservesReason(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	originalID, _, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/p.pdf"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.ClaimPending(ctx, originalID); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := repo.MarkFailed(ctx, originalID, domain.FailureReasonScannedPDF, "no text"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	_, prior, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/p2.pdf"))
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if prior == nil {
		t.Fatalf("prior = nil, want PriorState")
	}
	if prior.Status != domain.JobStatusFailed {
		t.Errorf("prior.Status = %q, want failed", prior.Status)
	}
	if prior.FailureReason != domain.FailureReasonScannedPDF {
		t.Errorf("prior.FailureReason = %q, want scanned_pdf", prior.FailureReason)
	}
}

func TestRepository_PeekNextPending_ReturnsOldestWithoutTransition(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	olderID, _, err := repo.Upsert(ctx, newPayload("older", "/tmp/older.pdf"))
	if err != nil {
		t.Fatalf("upsert older: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	newerID, _, err := repo.Upsert(ctx, newPayload("newer", "/tmp/newer.pdf"))
	if err != nil {
		t.Fatalf("upsert newer: %v", err)
	}
	_ = newerID

	row, ok, err := repo.PeekNextPending(ctx)
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true with two pending rows")
	}
	if row.ID != olderID {
		t.Errorf("peeked id = %q, want oldest %q", row.ID, olderID)
	}

	// The peek must NOT transition the row.
	got, err := repo.FindByID(ctx, olderID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Status != domain.JobStatusPending {
		t.Errorf("status after peek = %q, want still pending", got.Status)
	}
}

func TestRepository_PeekNextPending_EmptyQueue(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))

	row, ok, err := repo.PeekNextPending(context.Background())

	if err != nil {
		t.Errorf("err = %v, want nil on empty queue", err)
	}
	if ok {
		t.Errorf("ok = true, want false on empty queue")
	}
	if row != nil {
		t.Errorf("row = %+v, want nil on empty queue", row)
	}
}

func TestRepository_ClaimPending_TransitionsAndIsExclusive(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	id, _, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/p.pdf"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := repo.ClaimPending(ctx, id); err != nil {
		t.Fatalf("claim: %v", err)
	}

	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Status != domain.JobStatusRunning {
		t.Errorf("status = %q, want running after claim", got.Status)
	}

	if err := repo.ClaimPending(ctx, id); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("second claim err = %v, want ErrInvalidTransition", err)
	}
}

func TestRepository_MarkDone_FromRunningSucceeds(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	id, _, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/p.pdf"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.ClaimPending(ctx, id); err != nil {
		t.Fatalf("claim: %v", err)
	}

	artifact := domain.Artifact{
		Title:        "Title",
		BodyMarkdown: "# heading",
		Metadata:     domain.Metadata{ContentType: "paper", WordCount: 12},
	}
	if err := repo.MarkDone(ctx, id, artifact); err != nil {
		t.Fatalf("mark done: %v", err)
	}

	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Status != domain.JobStatusDone {
		t.Errorf("status = %q, want done", got.Status)
	}
	if got.Artifact == nil {
		t.Fatalf("artifact = nil, want populated")
	}
	if !reflect.DeepEqual(*got.Artifact, artifact) {
		t.Errorf("artifact = %+v, want %+v", *got.Artifact, artifact)
	}
}

func TestRepository_MarkDone_FromPendingRejected(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	id, _, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/p.pdf"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	err = repo.MarkDone(ctx, id, domain.Artifact{
		Title:        "x",
		BodyMarkdown: "y",
		Metadata:     domain.Metadata{ContentType: "paper", WordCount: 1},
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("err = %v, want ErrInvalidTransition (mark done on pending)", err)
	}
}

func TestRepository_MarkFailed_ExpiredFromPendingSucceeds(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	id, _, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/p.pdf"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := repo.MarkFailed(ctx, id, domain.FailureReasonExpired, "ttl elapsed"); err != nil {
		t.Fatalf("mark failed expired: %v", err)
	}

	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Status != domain.JobStatusFailed {
		t.Errorf("status = %q, want failed", got.Status)
	}
	if got.Failure == nil || got.Failure.Reason != domain.FailureReasonExpired {
		t.Errorf("failure = %+v, want reason=expired", got.Failure)
	}
}

func TestRepository_MarkFailed_ExpiredFromRunningRejected(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	id, _, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/p.pdf"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.ClaimPending(ctx, id); err != nil {
		t.Fatalf("claim: %v", err)
	}

	err = repo.MarkFailed(ctx, id, domain.FailureReasonExpired, "ttl elapsed")
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("err = %v, want ErrInvalidTransition (expired only valid from pending)", err)
	}
}

func TestRepository_MarkFailed_ExtractorFailureFromRunningSucceeds(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	id, _, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/p.pdf"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.ClaimPending(ctx, id); err != nil {
		t.Fatalf("claim: %v", err)
	}

	if err := repo.MarkFailed(ctx, id, domain.FailureReasonExtractorFailure, "boom"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Status != domain.JobStatusFailed {
		t.Errorf("status = %q, want failed", got.Status)
	}
	if got.Failure == nil || got.Failure.Reason != domain.FailureReasonExtractorFailure {
		t.Errorf("failure = %+v, want reason=extractor_failure", got.Failure)
	}
	if got.Failure.Message != "boom" {
		t.Errorf("failure.message = %q, want %q", got.Failure.Message, "boom")
	}
}

func TestRepository_MarkFailed_ExtractorFailureFromPendingRejected(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	id, _, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/p.pdf"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	err = repo.MarkFailed(ctx, id, domain.FailureReasonExtractorFailure, "boom")
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("err = %v, want ErrInvalidTransition (extractor_failure only valid from running)", err)
	}
}

func TestRepository_RecoverRunningOnStartup_FlipsRunningToFailedAndIsIdempotent(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	idA, _, err := repo.Upsert(ctx, newPayload("a", "/tmp/a.pdf"))
	if err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	idB, _, err := repo.Upsert(ctx, newPayload("b", "/tmp/b.pdf"))
	if err != nil {
		t.Fatalf("upsert b: %v", err)
	}
	for _, id := range []string{idA, idB} {
		if err := repo.ClaimPending(ctx, id); err != nil {
			t.Fatalf("claim %s: %v", id, err)
		}
	}

	recovered, err := repo.RecoverRunningOnStartup(ctx)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if recovered != 2 {
		t.Errorf("recovered = %d, want 2", recovered)
	}
	for _, id := range []string{idA, idB} {
		got, findErr := repo.FindByID(ctx, id)
		if findErr != nil {
			t.Fatalf("find %s: %v", id, findErr)
		}
		if got.Status != domain.JobStatusFailed {
			t.Errorf("status %s = %q, want failed", id, got.Status)
		}
		if got.Failure == nil || got.Failure.Reason != domain.FailureReasonProcessRestart {
			t.Errorf("failure %s = %+v, want reason=process_restart", id, got.Failure)
		}
	}

	// Idempotent: a second call observes zero rows.
	recovered2, err := repo.RecoverRunningOnStartup(ctx)
	if err != nil {
		t.Fatalf("recover (second): %v", err)
	}
	if recovered2 != 0 {
		t.Errorf("recovered (second) = %d, want 0", recovered2)
	}
}

func TestRepository_FindByID_Miss(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))

	got, err := repo.FindByID(context.Background(), "00000000-0000-4000-8000-deadbeefcafe")

	if got != nil {
		t.Errorf("entry = %+v, want nil", got)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRepository_ListPendingIDs_OldestFirstAndExcludesNonPending(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	idOlder, _, err := repo.Upsert(ctx, newPayload("older", "/tmp/o.pdf"))
	if err != nil {
		t.Fatalf("upsert older: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	idNewer, _, err := repo.Upsert(ctx, newPayload("newer", "/tmp/n.pdf"))
	if err != nil {
		t.Fatalf("upsert newer: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	idDone, _, err := repo.Upsert(ctx, newPayload("done", "/tmp/d.pdf"))
	if err != nil {
		t.Fatalf("upsert done: %v", err)
	}
	if err := repo.ClaimPending(ctx, idDone); err != nil {
		t.Fatalf("claim done: %v", err)
	}
	if err := repo.MarkDone(ctx, idDone, domain.Artifact{
		Title:        "t",
		BodyMarkdown: "b",
		Metadata:     domain.Metadata{ContentType: "paper", WordCount: 1},
	}); err != nil {
		t.Fatalf("mark done: %v", err)
	}

	ids, err := repo.ListPendingIDs(ctx)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	want := []string{idOlder, idNewer}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestRepository_ListPendingIDs_EmptyReturnsNonNilSlice(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))

	ids, err := repo.ListPendingIDs(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if ids == nil {
		t.Errorf("ids = nil, want non-nil empty slice")
	}
	if len(ids) != 0 {
		t.Errorf("len = %d, want 0", len(ids))
	}
}

func TestRepository_RequestPayloadJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	repo := extractionpersist.NewRepository(testdb.New(t))
	ctx := context.Background()

	want := domain.RequestPayload{
		SourceType: "paper",
		SourceID:   "2404.12345",
		PDFPath:    "/tmp/papers/with spaces/and \"quotes\".pdf",
	}
	id, _, err := repo.Upsert(ctx, want)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.RequestPayload != want {
		t.Errorf("payload roundtrip mismatch: got %+v, want %+v", got.RequestPayload, want)
	}
}

func TestRepository_FindByID_MalformedRequestPayloadSurfacesAsCatalogueUnavailable(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	repo := extractionpersist.NewRepository(db)
	ctx := context.Background()

	// Seed a valid row, then corrupt request_payload via raw GORM so ToDomain's
	// JSON unmarshal fails and the wrapper sentinel surfaces.
	id, _, err := repo.Upsert(ctx, newPayload("2404.12345", "/tmp/p.pdf"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.Exec(
		"UPDATE extractions SET request_payload = ? WHERE id = ?",
		"this-is-not-json",
		id,
	).Error; err != nil {
		t.Fatalf("corrupt row: %v", err)
	}

	got, err := repo.FindByID(ctx, id)
	if got != nil {
		t.Errorf("entry = %+v, want nil on malformed payload", got)
	}
	if !errors.Is(err, domain.ErrCatalogueUnavailable) {
		t.Errorf("err = %v, want wrapping ErrCatalogueUnavailable", err)
	}
}
