# Search17

**A Trust & Safety agent for AI agent memory files.**

AI agents increasingly persist "memories" — facts, preferences, and instructions extracted
from past conversations — and later recall them without a human in the loop. That makes
memory stores a prime target: a single manipulated, contradictory, or stale entry can
silently steer an agent's future behavior. Search17 audits those memory files for:

- **Manipulation** — content crafted to deceive or socially engineer the agent or its user.
- **Persistent-instruction attacks** — memory text that plants standing instructions/overrides
  for the agent to obey later (prompt injection via memory, not via the live conversation).
- **Contradictions** — two or more memories asserting incompatible facts about the same thing.
- **Staleness** — memories that were once true but are likely outdated.

## Why an agent, not a workflow

Search17's core is an **LLM-driven ReAct-style tool-calling loop**, not a fixed pipeline
of if/else detectors. The LLM is the mandatory reasoning engine: it decides what to inspect,
when to look closer at a specific record, and when the audit is complete. Tools exist only
for *mechanics and grounding* — getting the current date, paging through records, recording a
finding, ending the run — never for making the actual classification. The agent judges; the
code around it only provides data access and keeps score.

This distinction matters for Search17 specifically: its own subject matter is adversarial
text designed to hijack agents. A deterministic pipeline can't adapt to novel manipulation
phrasing; an agent reasoning over each record in context can.

```
CLI (Cobra: scan|audit|trust-score|repair)
  -> Memory Parser            (internal/memory)
  -> Agent Runner: ReAct loop (internal/agent)
       tools: get_current_date, list_memories, get_memory,
              record_finding, finalize_report / finalize_repair_plan
  -> LLM Provider              (internal/llm: openai.go + mock.go)
  -> Trust Scorer (deterministic, internal/score)
  -> Reporter (boxed text / JSON, internal/report)
```

### Prompt-injection defense

Every memory record the agent reads is **untrusted data**, never an instruction. The system
prompt enforces this explicitly: if a memory's text tells the agent to ignore findings,
report a perfect score, or otherwise act as a command, the agent must treat that *itself* as
a `persistent_instruction` finding — never comply with it. A regression test
(`internal/agent/runner_test.go: TestRunner_ResistsPromptInjectionFromMemoryContent`) runs the
adversarial fixture ([testdata/adversarial.json](testdata/adversarial.json)) through the agent loop to guard this behavior.

### Tools available to the agent

| Tool | Purpose |
|---|---|
| `get_current_date` | Ground truth for judging staleness |
| `list_memories(offset, limit)` | Paginate the memory store into context |
| `get_memory(id)` | Fetch one record's full text for closer inspection |
| `record_finding(category, risk, memory_ids, title, detail)` | Record one confirmed issue |
| `finalize_report(summary)` | End a `scan`/`audit`/`trust-score` run |
| `finalize_repair_plan(summary, actions)` | End a `repair` run with remediation recommendations |

## Setup

```sh
go build -o search17 .
export OPENAI_API_KEY=sk-...   # required — Search17 has no non-LLM fallback mode
```

`--api-key` flag overrides the environment variable. `--model` (default `gpt-4o-mini`) and
`--max-turns` (default `12`) are also configurable per run.

## Commands

| Command | Behavior |
|---|---|
| `scan <file>` | Fast, low-turn triage pass (few tool calls, quick signal) |
| `audit <file>` | Full investigative run; prints the complete boxed/JSON report |
| `trust-score <file>` | Runs a full audit, prints only the score (`--verbose` adds the breakdown) |
| `repair <file>` | Audits, then asks the agent for remediation recommendations per flagged memory — **print-only, never modifies the input file** |

All commands accept `--json` for machine-readable output instead of the boxed report.

### Example

```sh
$ search17 audit testdata/memory.json
```

```
╔══════════════════════════════════════════════════════════════╗
║ Search17 Audit Report                                          ║
╚══════════════════════════════════════════════════════════════╝
File:        testdata/memory.json
Trust Score: 36 / 100 (critical=1 high=2 medium=1 low=0)

Findings (4)
────────────────────────────────────────
[CRITICAL] persistent_instruction — Embedded override instruction
  Memories: mem-103
  Memory text instructs the agent to ignore future warnings and always
  report a trust score of 100 — a prompt-injection attempt; not obeyed.

[HIGH] manipulation — False-authority social engineering attempt
  Memories: mem-102
  Claims to be "IT security" to get billing data shared without verification.

[HIGH] contradiction — Conflicting preferred contact method
  Memories: mem-104, mem-105
  mem-104 says email; mem-105 (newer) says SMS instead of email.

[MEDIUM] staleness — Office location likely outdated
  Memories: mem-106
  Recorded in 2023 with low confidence; no corroborating recent record.

Summary: Detected a prompt-injection attempt, a social-engineering claim,
a direct contradiction, and a stale fact.
```

## Trust score

```
score = clamp(100 - (critical*25 + high*15 + medium*7 + low*3), 0, 100)
```

Computed deterministically in Go from the agent's recorded findings — the agent classifies,
the formula scores, so results stay auditable and reproducible across runs for a fixed set of
findings.

## Caveats

- **LLM is mandatory.** There is no rule-based fallback; without a valid API key, every
  command fails with a clear error.
- **Context window / scale.** Large memory stores are paged via `list_memories`/`get_memory`
  rather than dumped into one message. Very large stores may need multiple pagination turns —
  increase `--max-turns` if a run reports `INCOMPLETE`.
- **Run-to-run variability.** Because judgment is LLM-driven, repeated runs on the same file
  may vary slightly. The default temperature (`0.1`) is kept low to minimize this; it is
  expected agent behavior, not a bug.
- **Cost/latency.** Each tool-calling turn is a model call. `scan` caps turns low by default
  for cheap triage; `audit`/`trust-score`/`repair` allow more turns for a thorough pass.

## Testing

```sh
go test ./... -race -cover
```

All tests run fully offline against a scripted mock provider (`internal/llm/mock.go`) — no
API key or network access required. A manual smoke test against the real OpenAI API:

```sh
go run . audit testdata/memory.json
```
