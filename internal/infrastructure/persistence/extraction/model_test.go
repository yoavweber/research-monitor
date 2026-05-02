package extraction_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	extractionpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	"github.com/yoavweber/research-monitor/backend/tests/testdb"
)

// newDomainExtraction returns a populated domain Extraction with deterministic
// timestamps so round-trip assertions are not flaky on monotonic-clock
// differences. The Status / Artifact / Failure are caller-overridable per
// subtest.
func newDomainExtraction(status domain.JobStatus) domain.Extraction {
	created := time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)
	updated := created.Add(2 * time.Hour)
	return domain.Extraction{
		ID:         "fixed-id-123",
		SourceType: "paper",
		SourceID:   "2404.12345",
		Status:     status,
		RequestPayload: domain.RequestPayload{
			SourceType: "paper",
			SourceID:   "2404.12345",
			PDFPath:    "/tmp/papers/2404.12345.pdf",
		},
		CreatedAt: created,
		UpdatedAt: updated,
	}
}

func TestExtractionAutoMigrate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)

	type indexRow struct {
		Name string
	}
	var names []string
	if err := db.Raw(
		"SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'extractions' ORDER BY name",
	).Scan(&names).Error; err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}

	wantPresent := map[string]bool{
		"idx_extractions_source_source_id":   false,
		"idx_extractions_status_created_at":  false,
	}
	for _, n := range names {
		if _, ok := wantPresent[n]; ok {
			wantPresent[n] = true
		}
	}
	for n, ok := range wantPresent {
		if !ok {
			t.Fatalf("expected index %q on extractions, got %v", n, names)
		}
	}

	type pragmaRow struct {
		Seqno int
		Cid   int
		Name  string
	}

	cases := []struct {
		index string
		want  []string
	}{
		{"idx_extractions_source_source_id", []string{"source_type", "source_id"}},
		{"idx_extractions_status_created_at", []string{"status", "created_at"}},
	}
	for _, c := range cases {
		var rows []pragmaRow
		if err := db.Raw("PRAGMA index_info(" + quoteSQL(c.index) + ")").Scan(&rows).Error; err != nil {
			t.Fatalf("pragma index_info(%s): %v", c.index, err)
		}
		if len(rows) != len(c.want) {
			t.Fatalf("index %s: got %d cols, want %d (rows=%+v)", c.index, len(rows), len(c.want), rows)
		}
		for i, want := range c.want {
			if rows[i].Seqno != i {
				t.Fatalf("index %s col[%d] seqno = %d, want %d", c.index, i, rows[i].Seqno, i)
			}
			if rows[i].Name != want {
				t.Fatalf("index %s col[%d] name = %q, want %q", c.index, i, rows[i].Name, want)
			}
		}
	}
}

// quoteSQL is a tiny helper to inline the index name as a single-quoted SQL
// literal — `PRAGMA index_info(?)` does not accept positional bind parameters
// in SQLite.
func quoteSQL(s string) string {
	return "'" + s + "'"
}

func TestExtractionFromDomainToDomain_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("pending status round-trips with no artifact and no failure", func(t *testing.T) {
		t.Parallel()

		in := newDomainExtraction(domain.JobStatusPending)

		row, err := extractionpersist.FromDomain(&in)
		if err != nil {
			t.Fatalf("FromDomain: %v", err)
		}
		out, err := row.ToDomain()
		if err != nil {
			t.Fatalf("ToDomain: %v", err)
		}

		if out.Artifact != nil {
			t.Fatalf("Artifact = %+v, want nil for pending", out.Artifact)
		}
		if out.Failure != nil {
			t.Fatalf("Failure = %+v, want nil for pending", out.Failure)
		}
		if !reflect.DeepEqual(*out, in) {
			t.Fatalf("round-trip mismatch:\n got = %+v\nwant = %+v", *out, in)
		}
	})

	t.Run("running status round-trips with no artifact and no failure", func(t *testing.T) {
		t.Parallel()

		in := newDomainExtraction(domain.JobStatusRunning)

		row, err := extractionpersist.FromDomain(&in)
		if err != nil {
			t.Fatalf("FromDomain: %v", err)
		}
		out, err := row.ToDomain()
		if err != nil {
			t.Fatalf("ToDomain: %v", err)
		}

		if out.Artifact != nil {
			t.Fatalf("Artifact = %+v, want nil for running", out.Artifact)
		}
		if out.Failure != nil {
			t.Fatalf("Failure = %+v, want nil for running", out.Failure)
		}
		if !reflect.DeepEqual(*out, in) {
			t.Fatalf("round-trip mismatch:\n got = %+v\nwant = %+v", *out, in)
		}
	})

	t.Run("done status round-trips artifact and leaves failure nil", func(t *testing.T) {
		t.Parallel()

		in := newDomainExtraction(domain.JobStatusDone)
		in.Artifact = &domain.Artifact{
			Title:        "Sample Paper",
			BodyMarkdown: "# Sample Paper\n\nBody with $x = y$ math.",
			Metadata: domain.Metadata{
				ContentType: "paper",
				WordCount:   42,
			},
		}

		row, err := extractionpersist.FromDomain(&in)
		if err != nil {
			t.Fatalf("FromDomain: %v", err)
		}
		out, err := row.ToDomain()
		if err != nil {
			t.Fatalf("ToDomain: %v", err)
		}

		if out.Failure != nil {
			t.Fatalf("Failure = %+v, want nil for done", out.Failure)
		}
		if out.Artifact == nil {
			t.Fatalf("Artifact = nil, want populated for done")
		}
		if !reflect.DeepEqual(*out, in) {
			t.Fatalf("round-trip mismatch:\n got = %+v\nwant = %+v", *out, in)
		}
	})

	t.Run("failed status round-trips failure and leaves artifact nil", func(t *testing.T) {
		t.Parallel()

		in := newDomainExtraction(domain.JobStatusFailed)
		in.Failure = &domain.Failure{
			Reason:  domain.FailureReasonScannedPDF,
			Message: "no extractable text",
		}

		row, err := extractionpersist.FromDomain(&in)
		if err != nil {
			t.Fatalf("FromDomain: %v", err)
		}
		out, err := row.ToDomain()
		if err != nil {
			t.Fatalf("ToDomain: %v", err)
		}

		if out.Artifact != nil {
			t.Fatalf("Artifact = %+v, want nil for failed", out.Artifact)
		}
		if out.Failure == nil {
			t.Fatalf("Failure = nil, want populated for failed")
		}
		if !reflect.DeepEqual(*out, in) {
			t.Fatalf("round-trip mismatch:\n got = %+v\nwant = %+v", *out, in)
		}
	})
}

func TestExtractionFromDomain_AssignsUUIDOnEmptyID(t *testing.T) {
	t.Parallel()

	in := newDomainExtraction(domain.JobStatusPending)
	in.ID = ""

	row, err := extractionpersist.FromDomain(&in)
	if err != nil {
		t.Fatalf("FromDomain: %v", err)
	}

	if row.ID == "" {
		t.Fatalf("row.ID is empty, want generated UUID")
	}
	if _, err := uuid.Parse(row.ID); err != nil {
		t.Fatalf("row.ID = %q is not a valid UUID: %v", row.ID, err)
	}
}

func TestExtractionFromDomain_PreservesProvidedID(t *testing.T) {
	t.Parallel()

	in := newDomainExtraction(domain.JobStatusPending)
	in.ID = "00000000-0000-4000-8000-000000000abc"

	row, err := extractionpersist.FromDomain(&in)
	if err != nil {
		t.Fatalf("FromDomain: %v", err)
	}

	if row.ID != in.ID {
		t.Fatalf("row.ID = %q, want %q", row.ID, in.ID)
	}
}
