package extraction_test

import (
	"errors"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

func TestSubmitRequest_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		req     extraction.SubmitRequest
		wantErr error
	}{
		{
			name:    "valid paper request returns nil error",
			req:     extraction.SubmitRequest{SourceType: "paper", SourceID: "src-1", PDFPath: "/tmp/x.pdf"},
			wantErr: nil,
		},
		{
			name:    "rejects empty source_type as invalid request",
			req:     extraction.SubmitRequest{SourceType: "", SourceID: "src-1", PDFPath: "/tmp/x.pdf"},
			wantErr: extraction.ErrInvalidRequest,
		},
		{
			name:    "rejects whitespace-only source_type as invalid request",
			req:     extraction.SubmitRequest{SourceType: "   ", SourceID: "src-1", PDFPath: "/tmp/x.pdf"},
			wantErr: extraction.ErrInvalidRequest,
		},
		{
			name:    "rejects empty source_id as invalid request",
			req:     extraction.SubmitRequest{SourceType: "paper", SourceID: "", PDFPath: "/tmp/x.pdf"},
			wantErr: extraction.ErrInvalidRequest,
		},
		{
			name:    "rejects whitespace-only source_id as invalid request",
			req:     extraction.SubmitRequest{SourceType: "paper", SourceID: " ", PDFPath: "/tmp/x.pdf"},
			wantErr: extraction.ErrInvalidRequest,
		},
		{
			name:    "rejects empty pdf_path as invalid request",
			req:     extraction.SubmitRequest{SourceType: "paper", SourceID: "src-1", PDFPath: ""},
			wantErr: extraction.ErrInvalidRequest,
		},
		{
			name:    "rejects whitespace-only pdf_path as invalid request",
			req:     extraction.SubmitRequest{SourceType: "paper", SourceID: "src-1", PDFPath: "  "},
			wantErr: extraction.ErrInvalidRequest,
		},
		{
			name:    "rejects unsupported source_type html",
			req:     extraction.SubmitRequest{SourceType: "html", SourceID: "src-1", PDFPath: "/tmp/x.pdf"},
			wantErr: extraction.ErrUnsupportedSourceType,
		},
		{
			name:    "rejects unsupported source_type rss",
			req:     extraction.SubmitRequest{SourceType: "rss", SourceID: "src-1", PDFPath: "/tmp/x.pdf"},
			wantErr: extraction.ErrUnsupportedSourceType,
		},
		{
			name:    "case-sensitive PAPER is unsupported",
			req:     extraction.SubmitRequest{SourceType: "PAPER", SourceID: "src-1", PDFPath: "/tmp/x.pdf"},
			wantErr: extraction.ErrUnsupportedSourceType,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.req.Validate()

			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() returned unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() error = %v, want errors.Is(_, %v)", err, tc.wantErr)
			}
		})
	}
}
