package memory

import (
	"testing"
)

func TestParse_Valid(t *testing.T) {
	data := []byte(`[
		{"id":"mem-1","memory":"hello","source":"chat","timestamp":"2026-01-01T00:00:00Z","confidence":0.5,"memory_type":"fact"}
	]`)

	result, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no validation errors, got %v", result.Errors)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	if result.Records[0].ID != "mem-1" {
		t.Errorf("expected id mem-1, got %s", result.Records[0].ID)
	}
}

func TestParse_CollectsErrorsWithoutAbort(t *testing.T) {
	data := []byte(`[
		{"id":"ok-1","memory":"good","source":"chat","timestamp":"2026-01-01T00:00:00Z","confidence":0.5,"memory_type":"fact"},
		{"id":"","memory":"bad id","source":"chat","timestamp":"2026-01-01T00:00:00Z","confidence":0.5,"memory_type":"fact"},
		{"id":"ok-2","memory":"good too","source":"chat","timestamp":"2026-01-01T00:00:00Z","confidence":0.5,"memory_type":"fact"}
	]`)

	result, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(result.Records))
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 validation error, got %d: %v", len(result.Errors), result.Errors)
	}
}

func TestParse_ConfidenceOutOfRange(t *testing.T) {
	data := []byte(`[{"id":"mem-1","memory":"x","source":"chat","timestamp":"2026-01-01T00:00:00Z","confidence":1.5,"memory_type":"fact"}]`)

	result, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestParse_BadTimestamp(t *testing.T) {
	data := []byte(`[{"id":"mem-1","memory":"x","source":"chat","timestamp":"not-a-date","confidence":0.5,"memory_type":"fact"}]`)

	result, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestParse_InvalidJSONSyntax(t *testing.T) {
	data := []byte(`not json at all`)

	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected an error for invalid JSON syntax")
	}
}

func TestLoadFile_Fixtures(t *testing.T) {
	cases := []struct {
		path       string
		wantValid  int
		wantErrors int
	}{
		{"../../testdata/valid.json", 3, 0},
		{"../../testdata/invalid.json", 1, 4},
		{"../../testdata/mixed_risk.json", 4, 0},
		{"../../testdata/adversarial.json", 3, 0},
	}

	for _, c := range cases {
		result, err := LoadFile(c.path)
		if err != nil {
			t.Fatalf("LoadFile(%s): unexpected error: %v", c.path, err)
		}
		if len(result.Records) != c.wantValid {
			t.Errorf("LoadFile(%s): expected %d valid records, got %d", c.path, c.wantValid, len(result.Records))
		}
		if len(result.Errors) != c.wantErrors {
			t.Errorf("LoadFile(%s): expected %d errors, got %d: %v", c.path, c.wantErrors, len(result.Errors), result.Errors)
		}
	}
}

func TestLoadFile_MissingFile(t *testing.T) {
	_, err := LoadFile("../../testdata/does_not_exist.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
