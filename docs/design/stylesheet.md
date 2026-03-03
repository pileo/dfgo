# Stylesheet

**Package**: `internal/attractor/style`

A CSS-like system for configuring per-node LLM parameters (model, reasoning effort, provider) via selectors with specificity-based resolution.

## Syntax

```css
* {
    llm_model: gpt-4;
    reasoning_effort: medium;
}

.box {
    llm_model: claude-3;
}

#critical_step {
    reasoning_effort: high;
}
```

Rules are `selector { property: value; }` blocks. Properties are arbitrary key-value pairs — the stylesheet doesn't validate them.

## Selectors

| Syntax | Matches | Specificity |
|---|---|---|
| `*` | All nodes | 0 (lowest) |
| `.box` or `box` | Nodes with `shape="box"` | 1 |
| `#myNode` | Node with `ID="myNode"` | 2 (highest) |

A bare identifier (no prefix) is treated as a shape class selector.

## Resolution

`Stylesheet.Resolve(node)` returns the merged property map for a node. Rules are applied in specificity order (lowest first), so higher-specificity rules overwrite lower ones. Within the same specificity, later rules win.

```go
ss, err := style.ParseStylesheet(src)
if err != nil {
    // structural error: unclosed brace, empty selector, etc.
}
props := ss.Resolve(node)
// props["llm_model"] → "claude-3" (from .box, overrides * rule)
// props["reasoning_effort"] → "high" (from #critical_step, overrides * rule)
```

## Current Status

The stylesheet parser and resolver are implemented and tested. `ParseStylesheet` returns an error for structural problems (unclosed braces, empty selectors), and the `stylesheet_syntax` validation rule checks this during pipeline validation. The stylesheet is **not yet applied during execution** — this is a planned integration point (Phase 4) where resolved properties would be passed to handlers to configure LLM parameters per-node.
