# Edge Selection

**Package**: `internal/attractor/edge`

After a node executes, the engine must choose which outgoing edge to follow. The selector uses a 5-step priority cascade.

## Priority Order

```
Step 1: Condition match     ← edge has a condition and it evaluates true
Step 2: Preferred label     ← edge label matches outcome.PreferredLabel
Step 3: Suggested next IDs  ← edge target matches one of outcome.SuggestedNextIDs
Step 4: Highest weight      ← among unconditional edges, pick highest weight
Step 5: Declaration order   ← fallback: first edge as declared in DOT
```

The first step that produces a match wins. Within step 1, the first matching conditional edge (by declaration order) wins.

## How Each Step Works

### Step 1: Condition Match

Iterates outgoing edges in declaration order. For each edge with a `condition` attribute, parses and evaluates the condition against the current environment (outcome status, preferred label, context snapshot). First match wins.

```dot
A -> B [condition="outcome=SUCCESS"]   // taken if A succeeded
A -> C [condition="outcome=FAIL"]      // taken if A failed
```

### Step 2: Preferred Label

If the handler's outcome includes a `PreferredLabel` (e.g., `"yes"` from a human approval), the edge whose `label` attribute matches is selected.

```dot
review -> approve [label="yes"]
review -> reject  [label="no"]
// If handler returns PreferredLabel="no", the reject edge is taken
```

### Step 3: Suggested Next IDs

If the outcome includes `SuggestedNextIDs`, the list is iterated in priority order. For each ID, the first outgoing edge whose target matches is selected. This allows handlers to request specific destinations with fallback preferences.

### Step 4: Highest Weight (Unconditional)

Among edges with no `condition` attribute, the one with the highest `weight` is selected. Equal weights tie-break by declaration order.

```dot
A -> B [weight="1"]
A -> C [weight="10"]   // C wins
```

### Step 5: Declaration Order

If nothing else matches, the first outgoing edge (by DOT declaration order) is selected. This is the ultimate fallback and ensures deterministic behavior.

## API

```go
nextEdge := edge.Select(graph, currentNodeID, outcome, pipelineContext)
// Returns *model.Edge or nil (no outgoing edges)
```

Returns `nil` only when the node has zero outgoing edges (terminal nodes).
