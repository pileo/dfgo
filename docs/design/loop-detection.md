# Loop Detection

**Package**: `internal/agent/loop`

Detects repetitive tool call patterns in the agent loop. When the agent gets stuck in a cycle, the detector triggers a steering intervention.

## Mechanism

Each tool call is hashed to a SHA-256 signature from its name and arguments. Signatures are stored in a sliding window (default size 10).

After each new signature, the detector checks for three loop patterns:

| Pattern | Minimum History | Example |
|---|---|---|
| Period-1 | 3 identical | A A A |
| Period-2 | 4 alternating | A B A B |
| Period-3 | 6 repeating | A B C A B C |

Period-3 detection excludes the degenerate case where all three elements are the same (already caught by period-1).

## Usage

```go
d := loop.NewDetector(10)  // window size 10

looping := d.Record("read_file", `{"path":"main.go"}`)
// false — first occurrence

looping = d.Record("edit_file", `{"path":"main.go","old":"a","new":"b"}`)
// false — different tool

looping = d.Record("read_file", `{"path":"main.go"}`)
looping = d.Record("edit_file", `{"path":"main.go","old":"a","new":"b"}`)
// true — ABAB pattern detected
```

## Integration

The session calls `detector.Record()` before each tool execution. On detection, it injects a steering message:

> WARNING: Repetitive tool call pattern detected. Please try a different approach.

The detector can be reset with `d.Reset()` if the agent breaks out of the loop.
