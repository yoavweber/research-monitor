package pdf_test

import (
	"errors"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
)

func TestKeyValidate(t *testing.T) {
	t.Parallel()

	t.Run("happy path arxiv-style identifier returns no error", func(t *testing.T) {
		t.Parallel()

		key := pdf.Key{
			SourceType: "paper",
			SourceID:   "2404.12345v1",
			URL:        "http://arxiv.org/pdf/2404.12345v1",
		}

		err := key.Validate()

		if err != nil {
			t.Fatalf("expected nil error for valid key, got %v", err)
		}
	})

	t.Run("url containing forward slashes is allowed", func(t *testing.T) {
		t.Parallel()

		key := pdf.Key{
			SourceType: "paper",
			SourceID:   "2404.12345v1",
			URL:        "https://example.com/some/deep/path/file.pdf",
		}

		err := key.Validate()

		if err != nil {
			t.Fatalf("expected nil error for url with slashes, got %v", err)
		}
	})

	rejectionCases := []struct {
		name string
		key  pdf.Key
	}{
		{
			name: "empty source type is rejected",
			key:  pdf.Key{SourceType: "", SourceID: "2404.12345v1", URL: "http://arxiv.org/pdf/2404.12345v1"},
		},
		{
			name: "whitespace-only source type is rejected",
			key:  pdf.Key{SourceType: "   ", SourceID: "2404.12345v1", URL: "http://arxiv.org/pdf/2404.12345v1"},
		},
		{
			name: "empty source id is rejected",
			key:  pdf.Key{SourceType: "paper", SourceID: "", URL: "http://arxiv.org/pdf/2404.12345v1"},
		},
		{
			name: "whitespace-only source id is rejected",
			key:  pdf.Key{SourceType: "paper", SourceID: "\t\n ", URL: "http://arxiv.org/pdf/2404.12345v1"},
		},
		{
			name: "empty url is rejected",
			key:  pdf.Key{SourceType: "paper", SourceID: "2404.12345v1", URL: ""},
		},
		{
			name: "whitespace-only url is rejected",
			key:  pdf.Key{SourceType: "paper", SourceID: "2404.12345v1", URL: "   "},
		},
		{
			name: "source type containing dotdot traversal is rejected",
			key:  pdf.Key{SourceType: "..", SourceID: "2404.12345v1", URL: "http://arxiv.org/pdf/x"},
		},
		{
			name: "source type containing forward slash is rejected",
			key:  pdf.Key{SourceType: "paper/evil", SourceID: "2404.12345v1", URL: "http://arxiv.org/pdf/x"},
		},
		{
			name: "source type containing backslash is rejected",
			key:  pdf.Key{SourceType: "paper\\evil", SourceID: "2404.12345v1", URL: "http://arxiv.org/pdf/x"},
		},
		{
			name: "source id containing dotdot traversal is rejected",
			key:  pdf.Key{SourceType: "paper", SourceID: "../etc/passwd", URL: "http://arxiv.org/pdf/x"},
		},
		{
			name: "source id containing forward slash is rejected",
			key:  pdf.Key{SourceType: "paper", SourceID: "foo/bar", URL: "http://arxiv.org/pdf/x"},
		},
		{
			name: "source id containing backslash is rejected",
			key:  pdf.Key{SourceType: "paper", SourceID: "foo\\bar", URL: "http://arxiv.org/pdf/x"},
		},
	}

	for _, tc := range rejectionCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.key.Validate()

			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, pdf.ErrInvalidKey) {
				t.Fatalf("expected error to wrap ErrInvalidKey, got %v", err)
			}
		})
	}
}
