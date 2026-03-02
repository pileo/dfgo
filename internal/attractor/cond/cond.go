// Package cond implements a simple condition expression parser and evaluator
// for Attractor edge conditions.
//
// Syntax:
//
//	expr     = clause ("&&" clause)*
//	clause   = key op value | key
//	key      = ident ("." ident)*
//	op       = "=" | "!="
//	value    = ident | string
//
// A bare key (no operator) is a truthy check — true if the key exists and is non-empty.
// Keys prefixed with "context." are looked up in the pipeline context.
// The special key "outcome" matches against the stage outcome status.
// The special key "preferred_label" matches against the outcome's preferred label.
package cond

import (
	"fmt"
	"strings"
)

// Env provides values for condition evaluation.
type Env struct {
	Outcome        string            // e.g. "SUCCESS", "FAIL"
	PreferredLabel string            // outcome's preferred label
	Context        map[string]string // pipeline context snapshot
}

// Clause is a single condition clause.
type Clause struct {
	Key string // e.g. "outcome", "context.approval", "preferred_label"
	Op  string // "=", "!=", or "" (truthy check)
	Val string
}

// Expr is a conjunction of clauses (all must be true).
type Expr struct {
	Clauses []Clause
}

// Parse parses a condition expression string.
func Parse(s string) (Expr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Expr{}, nil
	}

	parts := splitAnd(s)
	var clauses []Clause
	for _, part := range parts {
		c, err := parseClause(strings.TrimSpace(part))
		if err != nil {
			return Expr{}, err
		}
		clauses = append(clauses, c)
	}
	return Expr{Clauses: clauses}, nil
}

func splitAnd(s string) []string {
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		if i+1 < len(s) && s[i] == '&' && s[i+1] == '&' {
			parts = append(parts, cur.String())
			cur.Reset()
			i++ // skip second &
			continue
		}
		cur.WriteByte(s[i])
	}
	parts = append(parts, cur.String())
	return parts
}

func parseClause(s string) (Clause, error) {
	if s == "" {
		return Clause{}, fmt.Errorf("empty clause")
	}

	// Try != first (before =)
	if idx := strings.Index(s, "!="); idx > 0 {
		key := strings.TrimSpace(s[:idx])
		val := strings.TrimSpace(s[idx+2:])
		if key == "" || val == "" {
			return Clause{}, fmt.Errorf("invalid clause: %q", s)
		}
		return Clause{Key: key, Op: "!=", Val: val}, nil
	}

	if idx := strings.Index(s, "="); idx > 0 {
		key := strings.TrimSpace(s[:idx])
		val := strings.TrimSpace(s[idx+1:])
		if key == "" || val == "" {
			return Clause{}, fmt.Errorf("invalid clause: %q", s)
		}
		return Clause{Key: key, Op: "=", Val: val}, nil
	}

	// Bare key = truthy check
	return Clause{Key: s}, nil
}

// Eval evaluates a parsed expression against an environment.
func (e Expr) Eval(env Env) bool {
	if len(e.Clauses) == 0 {
		return true // empty condition always matches
	}
	for _, c := range e.Clauses {
		if !evalClause(c, env) {
			return false
		}
	}
	return true
}

func evalClause(c Clause, env Env) bool {
	actual := resolve(c.Key, env)

	switch c.Op {
	case "=":
		return actual == c.Val
	case "!=":
		return actual != c.Val
	case "":
		// truthy: non-empty
		return actual != ""
	}
	return false
}

func resolve(key string, env Env) string {
	switch {
	case key == "outcome":
		return env.Outcome
	case key == "preferred_label":
		return env.PreferredLabel
	case strings.HasPrefix(key, "context."):
		ctxKey := key[len("context."):]
		return env.Context[ctxKey]
	default:
		// Try context as fallback
		return env.Context[key]
	}
}

// Validate checks that a condition string parses correctly without evaluating.
func Validate(s string) error {
	_, err := Parse(s)
	return err
}
