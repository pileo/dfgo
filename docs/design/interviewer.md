# Interviewer

**Package**: `internal/attractor/interviewer`

The interviewer system handles human interaction during pipeline execution (approval gates, review prompts, etc.).

## Interface

```go
type Interviewer interface {
    Ask(q Question) (Answer, error)
}

type Question struct {
    Type    QuestionType  // YesNo, MultipleChoice, Freeform, Confirmation
    Prompt  string
    Options []string      // for MultipleChoice
    Default string
}

type Answer struct {
    Text     string
    Selected int  // option index for MultipleChoice, -1 otherwise
}
```

## Implementations

### AutoApprove

Always answers affirmatively. Used for non-interactive/CI execution.

- Yes/No → `"yes"`
- MultipleChoice → first option
- Freeform → default value or `"yes"`

### Console

Interactive terminal I/O via stdin/stdout. Supports:

- Yes/No prompts: `Continue? [y/n]:`
- Multiple choice with numbered options
- Accelerator key input (type `Y` instead of `1` for `[Y] Yes`)
- Confirmation (Enter to confirm)
- Freeform with optional default

### Queue

Returns pre-filled answers in sequence. Used for testing.

```go
q := interviewer.NewQueue("yes", "no", "custom answer")
ans, err := q.Ask(question)  // returns "yes" first time, "no" second, etc.
q.Remaining()                // how many unused answers remain
```

Panics (returns error) if the queue is exhausted.

### Callback

Delegates to a user-supplied function. Designed for programmatic use when embedding dfgo as a library.

```go
cb := interviewer.NewCallback(func(q interviewer.Question) (interviewer.Answer, error) {
    return interviewer.Answer{Text: "approved", Selected: -1}, nil
})
ans, err := cb.Ask(question)  // calls the function
```

Composes naturally with `Recording` and other decorators.

### Recording

Wraps any other interviewer and records all question/answer pairs.

```go
rec := interviewer.NewRecording(inner)
rec.Ask(question)
fmt.Println(rec.Interactions)  // [{Question: ..., Answer: ...}]
```

Useful for debugging and test assertions.

## Accelerator Keys

Option labels can embed keyboard accelerators:

```
[Y] Yes    → accelerator "Y"
[N] No     → accelerator "N"
(A) Alpha  → accelerator "A"
B) Beta    → accelerator "B"
```

`ParseAccelerator(label)` extracts the key and cleaned label. `MatchAccelerator(input, options)` finds which option an input matches. The Console interviewer uses this so users can type `Y` instead of `1`.

## Integration with Handlers

The `WaitHumanHandler` uses the interviewer. It reads the node's `prompt` attribute, inspects outgoing edge labels to build multiple-choice options (if 2+ labeled edges exist), and sets the answer as `PreferredLabel` on the outcome. Edge selection then routes based on the human's choice.

```dot
review [shape=hexagon, type="wait.human", prompt="Approve the changes?"]
review -> next [label="approve"]
review -> redo [label="reject"]
```

The human sees: `Approve the changes?` with options `1) approve` and `2) reject`.
