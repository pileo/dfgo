# Condition Language

**Package**: `internal/attractor/cond`

A minimal expression language for edge conditions. Conditions determine which outgoing edge to follow after a stage completes.

## Syntax

```
expr     = clause ("&&" clause)*
clause   = key "=" value       -- equality
         | key "!=" value      -- inequality
         | key                 -- truthy (non-empty)
key      = identifier ("." identifier)*
value    = identifier
```

No `||` (or), no parentheses, no nesting. Conditions are conjunctions only — all clauses must be true.

## Keys

| Key pattern | Resolves to |
|---|---|
| `outcome` | Stage outcome status: `SUCCESS`, `FAIL`, `RETRY`, `PARTIAL_SUCCESS` |
| `preferred_label` | Outcome's preferred label (set by handler) |
| `context.{name}` | Value from pipeline context |
| `{anything_else}` | Falls back to pipeline context lookup |

## Examples

```dot
// Route on outcome
check -> path_a [condition="outcome=SUCCESS"]
check -> path_b [condition="outcome=FAIL"]

// Route on context value
gate -> approved [condition="context.review=approved"]
gate -> rejected [condition="context.review!=approved"]

// Truthy check (non-empty value)
gate -> ready [condition="context.data_loaded"]

// Conjunction
gate -> proceed [condition="outcome=SUCCESS && context.approved=true"]
```

## Evaluation

```go
expr, err := cond.Parse("outcome=SUCCESS && context.ready")
env := cond.Env{
    Outcome:        "SUCCESS",
    PreferredLabel: "yes",
    Context:        map[string]string{"ready": "true"},
}
result := expr.Eval(env)  // true
```

An empty condition always evaluates to `true`.

## Validation

The `validate` package's `condition_syntax` rule calls `cond.Validate()` on every edge condition during the validation phase, catching syntax errors before execution starts.
