package analyzer

import "testing"

func TestParseThesisEnvelope_Valid(t *testing.T) {
	t.Parallel()

	got, ok := parseThesisEnvelope(`{"flag": true, "rationale": "promising angle"}`)

	if !ok {
		t.Fatalf("ok = false for a well-formed envelope; got = %+v", got)
	}
	if !got.Flag {
		t.Errorf("Flag = false, want true")
	}
	if got.Rationale != "promising angle" {
		t.Errorf("Rationale = %q, want %q", got.Rationale, "promising angle")
	}
}

func TestParseThesisEnvelope_TrimsSurroundingWhitespace(t *testing.T) {
	t.Parallel()

	got, ok := parseThesisEnvelope("\n\t  {\"flag\": false, \"rationale\": \"  not yet  \"}  \n")

	if !ok {
		t.Fatalf("ok = false; got = %+v", got)
	}
	if got.Flag {
		t.Errorf("Flag = true, want false")
	}
	if got.Rationale != "not yet" {
		t.Errorf("Rationale = %q, want %q", got.Rationale, "not yet")
	}
}

func TestParseThesisEnvelope_ToleratesExtraFields(t *testing.T) {
	t.Parallel()

	got, ok := parseThesisEnvelope(`{"flag": true, "rationale": "ok", "extra": 42}`)

	if !ok || !got.Flag || got.Rationale != "ok" {
		t.Fatalf("envelope with extra fields rejected; got=%+v ok=%v", got, ok)
	}
}

func TestParseThesisEnvelope_RejectsNonJSON(t *testing.T) {
	t.Parallel()

	if _, ok := parseThesisEnvelope("definitely not json"); ok {
		t.Fatal("ok = true for non-JSON input")
	}
}

func TestParseThesisEnvelope_RejectsMissingFlag(t *testing.T) {
	t.Parallel()

	if _, ok := parseThesisEnvelope(`{"rationale": "no flag here"}`); ok {
		t.Fatal("ok = true for envelope missing flag")
	}
}

func TestParseThesisEnvelope_RejectsMissingRationale(t *testing.T) {
	t.Parallel()

	if _, ok := parseThesisEnvelope(`{"flag": true}`); ok {
		t.Fatal("ok = true for envelope missing rationale")
	}
}

func TestParseThesisEnvelope_RejectsNonBooleanFlag(t *testing.T) {
	t.Parallel()

	if _, ok := parseThesisEnvelope(`{"flag": "yes", "rationale": "bad type"}`); ok {
		t.Fatal("ok = true for envelope with non-boolean flag")
	}
}

func TestParseThesisEnvelope_RejectsNonStringRationale(t *testing.T) {
	t.Parallel()

	if _, ok := parseThesisEnvelope(`{"flag": true, "rationale": 12}`); ok {
		t.Fatal("ok = true for envelope with non-string rationale")
	}
}

func TestParseThesisEnvelope_RejectsEmptyRationale(t *testing.T) {
	t.Parallel()

	if _, ok := parseThesisEnvelope(`{"flag": true, "rationale": "   "}`); ok {
		t.Fatal("ok = true for envelope with whitespace-only rationale")
	}
}

func TestParseThesisEnvelope_RejectsEmptyInput(t *testing.T) {
	t.Parallel()

	if _, ok := parseThesisEnvelope(""); ok {
		t.Fatal("ok = true for empty input")
	}
}
