// Package dot implements a hand-rolled DOT lexer and recursive-descent parser
// for the restricted subset of Graphviz DOT used by Attractor pipelines.
package dot

import (
	"fmt"
	"strings"
	"unicode"
)

// TokenType identifies a DOT token kind.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenDigraph
	TokenSubgraph
	TokenLBrace
	TokenRBrace
	TokenLBracket
	TokenRBracket
	TokenArrow // ->
	TokenEquals
	TokenSemicolon
	TokenComma
	TokenIdent
	TokenString // quoted string
	TokenGraph  // "graph" keyword (for graph-level attrs)
	TokenNode   // "node" keyword (for default node attrs)
	TokenEdge   // "edge" keyword (for default edge attrs)
)

var tokenNames = map[TokenType]string{
	TokenEOF:       "EOF",
	TokenDigraph:   "digraph",
	TokenSubgraph:  "subgraph",
	TokenLBrace:    "{",
	TokenRBrace:    "}",
	TokenLBracket:  "[",
	TokenRBracket:  "]",
	TokenArrow:     "->",
	TokenEquals:    "=",
	TokenSemicolon: ";",
	TokenComma:     ",",
	TokenIdent:     "IDENT",
	TokenString:    "STRING",
	TokenGraph:     "graph",
	TokenNode:      "node",
	TokenEdge:      "edge",
}

func (t TokenType) String() string {
	if s, ok := tokenNames[t]; ok {
		return s
	}
	return fmt.Sprintf("TokenType(%d)", int(t))
}

// Token is a lexed DOT token.
type Token struct {
	Type TokenType
	Text string
	Line int
	Col  int
}

func (t Token) String() string {
	return fmt.Sprintf("%s(%q) at %d:%d", t.Type, t.Text, t.Line, t.Col)
}

// Lexer tokenizes DOT source text.
type Lexer struct {
	src  []rune
	pos  int
	line int
	col  int
}

// NewLexer creates a new Lexer for the given source.
func NewLexer(src string) *Lexer {
	return &Lexer{
		src:  []rune(src),
		line: 1,
		col:  1,
	}
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) advance() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.src) && unicode.IsSpace(l.src[l.pos]) {
		l.advance()
	}
}

func (l *Lexer) skipLineComment() {
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.advance()
	}
}

func (l *Lexer) skipBlockComment() {
	// Already past "/*"
	for l.pos < len(l.src)-1 {
		if l.src[l.pos] == '*' && l.src[l.pos+1] == '/' {
			l.advance() // *
			l.advance() // /
			return
		}
		l.advance()
	}
	// Consume remaining
	for l.pos < len(l.src) {
		l.advance()
	}
}

func (l *Lexer) skipWhitespaceAndComments() {
	for {
		l.skipWhitespace()
		if l.pos < len(l.src)-1 && l.src[l.pos] == '/' && l.src[l.pos+1] == '/' {
			l.advance()
			l.advance()
			l.skipLineComment()
			continue
		}
		if l.pos < len(l.src)-1 && l.src[l.pos] == '/' && l.src[l.pos+1] == '*' {
			l.advance()
			l.advance()
			l.skipBlockComment()
			continue
		}
		// Also handle # line comments
		if l.pos < len(l.src) && l.src[l.pos] == '#' {
			l.skipLineComment()
			continue
		}
		break
	}
}

func (l *Lexer) readString() (string, error) {
	// Already past opening quote
	var buf strings.Builder
	for l.pos < len(l.src) {
		ch := l.advance()
		if ch == '\\' && l.pos < len(l.src) {
			next := l.advance()
			switch next {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case '"':
				buf.WriteByte('"')
			case '\\':
				buf.WriteByte('\\')
			default:
				buf.WriteByte('\\')
				buf.WriteRune(next)
			}
			continue
		}
		if ch == '"' {
			return buf.String(), nil
		}
		buf.WriteRune(ch)
	}
	return "", fmt.Errorf("unterminated string at line %d", l.line)
}

func isIdentStart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func isIdentPart(ch rune) bool {
	return ch == '_' || ch == '.' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

func (l *Lexer) readIdent() string {
	var buf strings.Builder
	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		buf.WriteRune(l.advance())
	}
	return buf.String()
}

// keywords maps DOT keywords to token types.
var keywords = map[string]TokenType{
	"digraph":  TokenDigraph,
	"subgraph": TokenSubgraph,
	"graph":    TokenGraph,
	"node":     TokenNode,
	"edge":     TokenEdge,
}

// NextToken returns the next token from the input.
func (l *Lexer) NextToken() (Token, error) {
	l.skipWhitespaceAndComments()
	line, col := l.line, l.col

	if l.pos >= len(l.src) {
		return Token{Type: TokenEOF, Line: line, Col: col}, nil
	}

	ch := l.peek()

	switch {
	case ch == '{':
		l.advance()
		return Token{Type: TokenLBrace, Text: "{", Line: line, Col: col}, nil
	case ch == '}':
		l.advance()
		return Token{Type: TokenRBrace, Text: "}", Line: line, Col: col}, nil
	case ch == '[':
		l.advance()
		return Token{Type: TokenLBracket, Text: "[", Line: line, Col: col}, nil
	case ch == ']':
		l.advance()
		return Token{Type: TokenRBracket, Text: "]", Line: line, Col: col}, nil
	case ch == '=':
		l.advance()
		return Token{Type: TokenEquals, Text: "=", Line: line, Col: col}, nil
	case ch == ';':
		l.advance()
		return Token{Type: TokenSemicolon, Text: ";", Line: line, Col: col}, nil
	case ch == ',':
		l.advance()
		return Token{Type: TokenComma, Text: ",", Line: line, Col: col}, nil
	case ch == '-' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '>':
		l.advance()
		l.advance()
		return Token{Type: TokenArrow, Text: "->", Line: line, Col: col}, nil
	case ch == '"':
		l.advance() // skip opening quote
		s, err := l.readString()
		if err != nil {
			return Token{}, err
		}
		return Token{Type: TokenString, Text: s, Line: line, Col: col}, nil
	case isIdentStart(ch):
		id := l.readIdent()
		if tt, ok := keywords[id]; ok {
			return Token{Type: tt, Text: id, Line: line, Col: col}, nil
		}
		return Token{Type: TokenIdent, Text: id, Line: line, Col: col}, nil
	case unicode.IsDigit(ch):
		// Numeric IDs: read as ident
		var buf strings.Builder
		for l.pos < len(l.src) && (unicode.IsDigit(l.src[l.pos]) || l.src[l.pos] == '.') {
			buf.WriteRune(l.advance())
		}
		return Token{Type: TokenIdent, Text: buf.String(), Line: line, Col: col}, nil
	}

	return Token{}, fmt.Errorf("unexpected character %q at %d:%d", ch, line, col)
}

// Tokenize returns all tokens from the source.
func Tokenize(src string) ([]Token, error) {
	l := NewLexer(src)
	var tokens []Token
	for {
		tok, err := l.NextToken()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}
	return tokens, nil
}
