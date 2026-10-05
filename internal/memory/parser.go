package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// ValidationError describes a single malformed record found while parsing a
// memory file. Parsing never aborts on the first bad record — all errors are
// collected so the caller can decide how to proceed.
type ValidationError struct {
	Index   int    // position of the record in the source array
	ID      string // best-effort record ID, may be empty if missing
	Message string
}

func (e ValidationError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("record %d (id=%s): %s", e.Index, e.ID, e.Message)
	}
	return fmt.Sprintf("record %d: %s", e.Index, e.Message)
}

// ParseResult holds the records that passed validation plus any per-record
// errors encountered along the way.
type ParseResult struct {
	Records []Record
	Errors  []ValidationError
}

// rawRecord mirrors Record but keeps Timestamp as a string so a single
// malformed field doesn't fail the whole array decode.
type rawRecord struct {
	ID         string  `json:"id"`
	Memory     string  `json:"memory"`
	Source     string  `json:"source"`
	Timestamp  string  `json:"timestamp"`
	Confidence float64 `json:"confidence"`
	MemoryType string  `json:"memory_type"`
}

// LoadFile reads and parses a memory file from disk.
func LoadFile(path string) (*ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading memory file: %w", err)
	}
	return Parse(data)
}

// Parse decodes raw JSON bytes into a ParseResult, validating each record
// independently so one bad record doesn't prevent the rest from loading.
func Parse(data []byte) (*ParseResult, error) {
	var raws []rawRecord
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("invalid memory file JSON: %w", err)
	}

	result := &ParseResult{}
	for i, raw := range raws {
		rec, verrs := validate(i, raw)
		result.Errors = append(result.Errors, verrs...)
		if len(verrs) == 0 {
			result.Records = append(result.Records, rec)
		}
	}
	return result, nil
}

func validate(index int, raw rawRecord) (Record, []ValidationError) {
	var errs []ValidationError
	fail := func(msg string) {
		errs = append(errs, ValidationError{Index: index, ID: raw.ID, Message: msg})
	}

	if raw.ID == "" {
		fail("missing required field 'id'")
	}
	if raw.Memory == "" {
		fail("missing required field 'memory'")
	}
	if raw.Source == "" {
		fail("missing required field 'source'")
	}
	if raw.MemoryType == "" {
		fail("missing required field 'memory_type'")
	}
	if raw.Confidence < 0 || raw.Confidence > 1 {
		fail(fmt.Sprintf("confidence %v out of range [0,1]", raw.Confidence))
	}

	var ts time.Time
	if raw.Timestamp == "" {
		fail("missing required field 'timestamp'")
	} else {
		parsed, err := time.Parse(time.RFC3339, raw.Timestamp)
		if err != nil {
			fail(fmt.Sprintf("invalid timestamp %q: must be RFC3339", raw.Timestamp))
		} else {
			ts = parsed
		}
	}

	if len(errs) > 0 {
		return Record{}, errs
	}

	return Record{
		ID:         raw.ID,
		Memory:     raw.Memory,
		Source:     raw.Source,
		Timestamp:  ts,
		Confidence: raw.Confidence,
		MemoryType: raw.MemoryType,
	}, nil
}
