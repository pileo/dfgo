# DOT Parser

**Package**: `internal/attractor/dot`

A hand-rolled lexer and recursive-descent parser for the restricted DOT subset used by Attractor pipelines. ~300 lines total across two files.

## Why Custom

- The DOT subset needed is small (digraphs only, no HTML labels, no port syntax)
- Better error messages with line/column info
- No dependency on gonum/graph or other DOT libraries
- Full control over attribute handling and node merging

## Lexer (`lexer.go`)

Tokenizes DOT source into a stream of typed tokens.

### Token Types

```
DIGRAPH  SUBGRAPH  GRAPH  NODE  EDGE
LBRACE   RBRACE    LBRACKET  RBRACKET
ARROW(->)  EQUALS  SEMICOLON  COMMA
IDENT    STRING    EOF
```

### Features

- **Quoted strings** with escape sequences: `\"`, `\\`, `\n`, `\t`
- **Three comment styles**: `// line`, `/* block */`, `# hash`
- **Numeric identifiers**: treated as `IDENT` tokens
- **DOT keyword recognition**: `digraph`, `subgraph`, `graph`, `node`, `edge`
- **Position tracking**: every token carries line and column numbers

### Usage

```go
tokens, err := dot.Tokenize(`digraph G { A -> B [label="hello"] }`)
// Or stream tokens one at a time:
lexer := dot.NewLexer(src)
tok, err := lexer.NextToken()
```

## Parser (`parser.go`)

Recursive-descent parser that builds a `model.Graph` from a token stream.

### Grammar (simplified)

```
digraph    = "digraph" ID? "{" stmt_list "}"
stmt_list  = (stmt ";"?)*
stmt       = graph_attr | node_attr | edge_attr | subgraph | node_or_edge
graph_attr = "graph" attr_list | ID "=" value
node_attr  = "node" attr_list
edge_attr  = "edge" attr_list
subgraph   = "subgraph" ID? "{" stmt_list "}"
node_or_edge = ID attr_list?          -- node statement
             | ID "->" ID ("->" ID)* attr_list?  -- edge chain
attr_list  = "[" (ID "=" value ","?)* "]"
```

### Key Behaviors

**Edge chaining**: `A -> B -> C` produces two edges (`A→B`, `B→C`) sharing the same attribute list.

**Implicit node creation**: nodes referenced in edge statements are auto-created if they don't already exist.

**Default attributes**: `node [shape=box]` sets defaults applied to all subsequent node declarations in that scope. Subgraphs inherit parent defaults.

**Subgraph flattening**: subgraph contents are merged into the parent graph. Subgraph names are ignored — they only serve as scoping for default attributes.

**Graph-level attributes**: supported both via `graph [key=val]` and bare `key = val` assignment.

**Node merging**: when a node ID appears multiple times, attributes are merged (see graph-model.md).

### Usage

```go
graph, err := dot.Parse(`digraph pipeline {
    start [shape=Mdiamond]
    A [shape=box, type="codergen", prompt="Do something"]
    exit [shape=Msquare]
    start -> A -> exit
}`)
```

### Error Reporting

Parse errors include line and column:

```
expected }, got IDENT at 5:3
```

Lexer errors report position for unterminated strings and unexpected characters.
