package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/search17/search17/internal/llm"
)

// resetPersistentFlags clears cobra's per-flag "Changed" tracking and value
// between test runs, since rootCmd and its subcommands are package-level
// singletons shared across tests.
func resetPersistentFlags() {
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
	})
	trustScoreCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
	})
}

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func runCLI(t *testing.T, args []string) (string, error) {
	t.Helper()
	resetPersistentFlags()
	rootCmd.SetArgs(args)
	var runErr error
	stdout := captureStdout(func() {
		runErr = rootCmd.Execute()
	})
	return stdout, runErr
}

func toolCallMsg(id, name string, args any) llm.Message {
	raw, _ := json.Marshal(args)
	return llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{ID: id, Name: name, Arguments: string(raw)}},
	}
}

func withMockProvider(t *testing.T, mock *llm.MockProvider) {
	t.Helper()
	old := newProvider
	newProvider = func(string) llm.Provider { return mock }
	t.Cleanup(func() { newProvider = old })
}

func TestCLI_Scan_NoFindings(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	withMockProvider(t, &llm.MockProvider{
		Responses: []llm.ChatResponse{
			{Message: toolCallMsg("c1", "finalize_report", map[string]any{"summary": "Nothing to flag."})},
		},
	})

	out, err := runCLI(t, []string{"scan", "../testdata/valid.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Trust Score: 100") {
		t.Errorf("expected trust score 100, got: %s", out)
	}
	if !strings.Contains(out, "No issues found.") {
		t.Errorf("expected no-issues message, got: %s", out)
	}
}

func TestCLI_Audit_JSONOutput(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	withMockProvider(t, &llm.MockProvider{
		Responses: []llm.ChatResponse{
			{Message: toolCallMsg("c1", "record_finding", map[string]any{
				"category": "staleness", "risk": "medium",
				"memory_ids": []string{"mem-003"}, "title": "Old deadline", "detail": "Deadline may have passed.",
			})},
			{Message: toolCallMsg("c2", "finalize_report", map[string]any{"summary": "One stale record."})},
		},
	})

	out, err := runCLI(t, []string{"audit", "../testdata/valid.json", "--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON output, got error %v:\n%s", err, out)
	}
	if parsed["trust_score"].(float64) != 93 {
		t.Errorf("expected trust score 93, got %v", parsed["trust_score"])
	}
}

func TestCLI_TrustScore_PrintsNumberOnly(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	withMockProvider(t, &llm.MockProvider{
		Responses: []llm.ChatResponse{
			{Message: toolCallMsg("c1", "finalize_report", map[string]any{"summary": "clean"})},
		},
	})

	out, err := runCLI(t, []string{"trust-score", "../testdata/valid.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if strings.TrimSpace(out) != "100" {
		t.Errorf("expected bare score 100, got: %q", out)
	}
}

func TestCLI_TrustScore_Verbose(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	withMockProvider(t, &llm.MockProvider{
		Responses: []llm.ChatResponse{
			{Message: toolCallMsg("c1", "record_finding", map[string]any{
				"category": "manipulation", "risk": "high",
				"memory_ids": []string{"mem-001"}, "title": "t", "detail": "d",
			})},
			{Message: toolCallMsg("c2", "finalize_report", map[string]any{"summary": "s"})},
		},
	})

	out, err := runCLI(t, []string{"trust-score", "../testdata/valid.json", "--verbose"})
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "high=1") {
		t.Errorf("expected breakdown with high=1, got: %s", out)
	}
}

func TestCLI_Repair_PrintsRemediationPlan(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	withMockProvider(t, &llm.MockProvider{
		Responses: []llm.ChatResponse{
			{Message: toolCallMsg("c1", "finalize_repair_plan", map[string]any{
				"summary": "One record needs quarantine.",
				"actions": []map[string]any{
					{"memory_id": "mem-001", "recommendation": "quarantine", "detail": "Suspicious."},
				},
			})},
		},
	})

	out, err := runCLI(t, []string{"repair", "../testdata/valid.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Remediation Plan") || !strings.Contains(out, "QUARANTINE") {
		t.Errorf("expected remediation plan output, got: %s", out)
	}
}

func TestCLI_MissingFile(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	withMockProvider(t, &llm.MockProvider{})

	_, err := runCLI(t, []string{"scan", "../testdata/does_not_exist.json"})
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestCLI_MalformedJSON(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	withMockProvider(t, &llm.MockProvider{})

	_, err := runCLI(t, []string{"audit", "../testdata/malformed.json"})
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestCLI_MissingAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	withMockProvider(t, &llm.MockProvider{})

	_, err := runCLI(t, []string{"audit", "../testdata/valid.json"})
	if err == nil || !strings.Contains(err.Error(), "no API key configured") {
		t.Fatalf("expected missing API key error, got: %v", err)
	}
}

func TestCLI_Scan_FromStdin(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	withMockProvider(t, &llm.MockProvider{
		Responses: []llm.ChatResponse{
			{Message: toolCallMsg("c1", "finalize_report", map[string]any{"summary": "clean"})},
		},
	})

	data, err := os.ReadFile("../testdata/valid.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	r, w, _ := os.Pipe()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })
	go func() {
		w.Write(data)
		w.Close()
	}()

	out, err := runCLI(t, []string{"scan", "-"})
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "<stdin>") {
		t.Errorf("expected report to reference <stdin>, got: %s", out)
	}
}
