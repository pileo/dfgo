package dot

import (
	"fmt"

	"dfgo/internal/attractor/model"
)

// Parser is a recursive-descent parser for DOT digraphs.
type Parser struct {
	lexer     *Lexer
	cur       Token
	nodeOrder int
	edgeOrder int
}

// Parse parses a DOT source string into a Graph.
func Parse(src string) (*model.Graph, error) {
	p := &Parser{lexer: NewLexer(src)}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return p.parseDigraph()
}

func (p *Parser) advance() error {
	tok, err := p.lexer.NextToken()
	if err != nil {
		return err
	}
	p.cur = tok
	return nil
}

func (p *Parser) expect(tt TokenType) (Token, error) {
	if p.cur.Type != tt {
		return Token{}, fmt.Errorf("expected %s, got %s at %d:%d", tt, p.cur.Type, p.cur.Line, p.cur.Col)
	}
	tok := p.cur
	if err := p.advance(); err != nil {
		return Token{}, err
	}
	return tok, nil
}

func (p *Parser) parseDigraph() (*model.Graph, error) {
	if _, err := p.expect(TokenDigraph); err != nil {
		return nil, err
	}

	name := ""
	if p.cur.Type == TokenIdent || p.cur.Type == TokenString {
		name = p.cur.Text
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(TokenLBrace); err != nil {
		return nil, err
	}

	g := model.NewGraph(name)

	if err := p.parseStmtList(g, nil); err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenRBrace); err != nil {
		return nil, err
	}

	return g, nil
}

func (p *Parser) parseStmtList(g *model.Graph, defaultAttrs map[string]map[string]string) error {
	if defaultAttrs == nil {
		defaultAttrs = map[string]map[string]string{
			"node": {},
			"edge": {},
		}
	}
	for p.cur.Type != TokenRBrace && p.cur.Type != TokenEOF {
		if err := p.parseStmt(g, defaultAttrs); err != nil {
			return err
		}
		// optional semicolons
		for p.cur.Type == TokenSemicolon {
			if err := p.advance(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Parser) parseStmt(g *model.Graph, defaultAttrs map[string]map[string]string) error {
	switch p.cur.Type {
	case TokenGraph:
		// graph-level attributes: graph [key=val, ...]
		if err := p.advance(); err != nil {
			return err
		}
		if p.cur.Type == TokenLBracket {
			attrs, err := p.parseAttrList()
			if err != nil {
				return err
			}
			for k, v := range attrs {
				g.Attrs[k] = v
			}
		}
		return nil

	case TokenNode:
		// default node attrs: node [key=val, ...]
		if err := p.advance(); err != nil {
			return err
		}
		if p.cur.Type == TokenLBracket {
			attrs, err := p.parseAttrList()
			if err != nil {
				return err
			}
			for k, v := range attrs {
				defaultAttrs["node"][k] = v
			}
		}
		return nil

	case TokenEdge:
		// default edge attrs: edge [key=val, ...]
		if err := p.advance(); err != nil {
			return err
		}
		if p.cur.Type == TokenLBracket {
			attrs, err := p.parseAttrList()
			if err != nil {
				return err
			}
			for k, v := range attrs {
				defaultAttrs["edge"][k] = v
			}
		}
		return nil

	case TokenSubgraph:
		return p.parseSubgraph(g, defaultAttrs)

	case TokenIdent, TokenString:
		return p.parseNodeOrEdgeStmt(g, defaultAttrs)

	default:
		// Graph-level attr: key = value
		return fmt.Errorf("unexpected token %s at %d:%d", p.cur.Type, p.cur.Line, p.cur.Col)
	}
}

func (p *Parser) parseSubgraph(g *model.Graph, parentDefaults map[string]map[string]string) error {
	if _, err := p.expect(TokenSubgraph); err != nil {
		return err
	}
	// Optional subgraph name
	if p.cur.Type == TokenIdent || p.cur.Type == TokenString {
		if err := p.advance(); err != nil {
			return err
		}
	}
	if _, err := p.expect(TokenLBrace); err != nil {
		return err
	}

	// Subgraph inherits parent defaults
	childDefaults := map[string]map[string]string{
		"node": copyMap(parentDefaults["node"]),
		"edge": copyMap(parentDefaults["edge"]),
	}

	if err := p.parseStmtList(g, childDefaults); err != nil {
		return err
	}

	if _, err := p.expect(TokenRBrace); err != nil {
		return err
	}
	return nil
}

func (p *Parser) parseNodeOrEdgeStmt(g *model.Graph, defaultAttrs map[string]map[string]string) error {
	id := p.cur.Text
	if err := p.advance(); err != nil {
		return err
	}

	// Check if this is a graph-level attribute: key = value
	if p.cur.Type == TokenEquals {
		if err := p.advance(); err != nil {
			return err
		}
		if p.cur.Type != TokenIdent && p.cur.Type != TokenString {
			return fmt.Errorf("expected value after '=' at %d:%d", p.cur.Line, p.cur.Col)
		}
		g.Attrs[id] = p.cur.Text
		return p.advance()
	}

	if p.cur.Type == TokenArrow {
		return p.parseEdgeStmt(g, id, defaultAttrs)
	}

	// Node statement
	attrs := copyMap(defaultAttrs["node"])
	if p.cur.Type == TokenLBracket {
		nodeAttrs, err := p.parseAttrList()
		if err != nil {
			return err
		}
		for k, v := range nodeAttrs {
			attrs[k] = v
		}
	}

	g.AddNode(&model.Node{
		ID:    id,
		Attrs: attrs,
		Order: p.nodeOrder,
	})
	p.nodeOrder++
	return nil
}

func (p *Parser) parseEdgeStmt(g *model.Graph, firstID string, defaultAttrs map[string]map[string]string) error {
	// Collect chain: A -> B -> C
	ids := []string{firstID}
	for p.cur.Type == TokenArrow {
		if err := p.advance(); err != nil {
			return err
		}
		if p.cur.Type != TokenIdent && p.cur.Type != TokenString {
			return fmt.Errorf("expected node ID after '->' at %d:%d", p.cur.Line, p.cur.Col)
		}
		ids = append(ids, p.cur.Text)
		if err := p.advance(); err != nil {
			return err
		}
	}

	// Optional attrs apply to all edges in chain
	attrs := copyMap(defaultAttrs["edge"])
	if p.cur.Type == TokenLBracket {
		edgeAttrs, err := p.parseAttrList()
		if err != nil {
			return err
		}
		for k, v := range edgeAttrs {
			attrs[k] = v
		}
	}

	// Ensure all nodes in the chain exist
	for _, id := range ids {
		if g.NodeByID(id) == nil {
			nodeAttrs := copyMap(defaultAttrs["node"])
			g.AddNode(&model.Node{
				ID:    id,
				Attrs: nodeAttrs,
				Order: p.nodeOrder,
			})
			p.nodeOrder++
		}
	}

	// Create edges for each pair
	for i := 0; i < len(ids)-1; i++ {
		g.AddEdge(&model.Edge{
			From:  ids[i],
			To:    ids[i+1],
			Attrs: copyMap(attrs),
			Order: p.edgeOrder,
		})
		p.edgeOrder++
	}

	return nil
}

func (p *Parser) parseAttrList() (map[string]string, error) {
	if _, err := p.expect(TokenLBracket); err != nil {
		return nil, err
	}

	attrs := make(map[string]string)
	for p.cur.Type != TokenRBracket && p.cur.Type != TokenEOF {
		if p.cur.Type != TokenIdent && p.cur.Type != TokenString {
			return nil, fmt.Errorf("expected attribute name, got %s at %d:%d", p.cur.Type, p.cur.Line, p.cur.Col)
		}
		key := p.cur.Text
		if err := p.advance(); err != nil {
			return nil, err
		}
		if _, err := p.expect(TokenEquals); err != nil {
			return nil, err
		}
		if p.cur.Type != TokenIdent && p.cur.Type != TokenString {
			return nil, fmt.Errorf("expected attribute value, got %s at %d:%d", p.cur.Type, p.cur.Line, p.cur.Col)
		}
		attrs[key] = p.cur.Text
		if err := p.advance(); err != nil {
			return nil, err
		}

		// optional comma/semicolon separator
		if p.cur.Type == TokenComma || p.cur.Type == TokenSemicolon {
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
	}

	if _, err := p.expect(TokenRBracket); err != nil {
		return nil, err
	}
	return attrs, nil
}

func copyMap(m map[string]string) map[string]string {
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
