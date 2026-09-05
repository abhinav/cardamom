// Package searchquery parses Cardamom's public full-text search language.
//
// Adjacent terms form an implicit AND. The uppercase operators NOT and OR are
// infix operators. Implicit AND binds more tightly than NOT, NOT binds more
// tightly than OR, and parentheses override that precedence.
//
// The grammar is:
//
//	expression  = difference { "OR" difference } .
//	difference  = conjunction { "NOT" conjunction } .
//	conjunction = primary { primary } .
//	primary     = term | phrase | "(" expression ")" .
//	term        = text [ "*" ] .
//	phrase      = '"' phrase-text '"' .
//
// Terms end at whitespace, parentheses, or a double quote. A trailing * asks
// for prefix matching and is not accepted elsewhere in a term. Phrases may
// contain whitespace and represent a double quote by doubling it. Blank
// expressions and phrases, unmatched parentheses or quotes, misplaced
// operators, and invalid prefix operators are rejected.
//
// Parse interprets this grammar. Literal instead treats its complete input as
// one phrase, so operators and grouping characters have no special meaning.
// Both functions return a Query whose Expression is normalized for repository
// search without exposing the storage engine's query language to callers.
package searchquery

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Query is a validated full-text expression in Cardamom's public query
// language.
type Query struct {
	expression string
}

// Parse validates an expression with implicit AND, infix OR and
// NOT, phrases, grouping, and trailing prefix operators.
func Parse(value string) (Query, error) {
	tokens, err := lex(value)
	if err != nil {
		return Query{}, err
	}
	parser := parser{tokens: tokens}
	node, err := parser.parseExpression()
	if err != nil {
		return Query{}, err
	}
	if parser.peek().kind != tokenEnd {
		return Query{}, invalidQuery("unexpected %q", parser.peek().text)
	}
	return Query{expression: node.expression()}, nil
}

// Literal creates a phrase query from text without interpreting operators or
// grouping characters.
func Literal(value string) (Query, error) {
	if strings.TrimSpace(value) == "" {
		return Query{}, errors.New("search query must not be blank")
	}
	return Query{expression: quoteText(value)}, nil
}

// Expression returns the normalized Cardamom search expression.
func (q Query) Expression() string { return q.expression }

type tokenKind uint8

const (
	tokenEnd tokenKind = iota
	tokenTerm
	tokenPhrase
	tokenLeftParenthesis
	tokenRightParenthesis
	tokenOr
	tokenNot
)

type token struct {
	kind   tokenKind
	text   string
	prefix bool
}

func lex(value string) ([]token, error) {
	var tokens []token
	for offset := 0; offset < len(value); {
		character, size := utf8.DecodeRuneInString(value[offset:])
		if unicode.IsSpace(character) {
			offset += size
			continue
		}
		switch character {
		case '(':
			tokens = append(tokens, token{kind: tokenLeftParenthesis, text: "("})
			offset += size
		case ')':
			tokens = append(tokens, token{kind: tokenRightParenthesis, text: ")"})
			offset += size
		case '"':
			phrase, next, err := scanPhrase(value, offset+size)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: tokenPhrase, text: phrase})
			offset = next
		default:
			term, next, err := scanTerm(value, offset)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, term)
			offset = next
		}
	}
	if len(tokens) == 0 {
		return nil, invalidQuery("expression is blank")
	}
	tokens = append(tokens, token{kind: tokenEnd})
	return tokens, nil
}

func scanPhrase(value string, offset int) (string, int, error) {
	var phrase strings.Builder
	for offset < len(value) {
		if value[offset] == '"' {
			if offset+1 < len(value) && value[offset+1] == '"' {
				phrase.WriteByte('"')
				offset += 2
				continue
			}
			if strings.TrimSpace(phrase.String()) == "" {
				return "", 0, invalidQuery("phrase is blank")
			}
			return phrase.String(), offset + 1, nil
		}
		character, size := utf8.DecodeRuneInString(value[offset:])
		phrase.WriteRune(character)
		offset += size
	}
	return "", 0, invalidQuery("phrase is not closed")
}

func scanTerm(value string, offset int) (token, int, error) {
	start := offset
	for offset < len(value) {
		character, size := utf8.DecodeRuneInString(value[offset:])
		if unicode.IsSpace(character) || strings.ContainsRune(`()"`, character) {
			break
		}
		offset += size
	}
	text := value[start:offset]
	switch text {
	case "OR":
		return token{kind: tokenOr, text: text}, offset, nil
	case "NOT":
		return token{kind: tokenNot, text: text}, offset, nil
	}
	prefix := strings.HasSuffix(text, "*")
	if strings.Contains(text, "*") {
		if !prefix || strings.Count(text, "*") != 1 || len(text) == 1 {
			return token{}, 0, invalidQuery("prefix operator must follow one term")
		}
		text = strings.TrimSuffix(text, "*")
	}
	return token{kind: tokenTerm, text: text, prefix: prefix}, offset, nil
}

type expression interface {
	expression() string
}

type textExpression struct {
	text   string
	prefix bool
}

func (e textExpression) expression() string {
	value := quoteText(e.text)
	if e.prefix {
		value += "*"
	}
	return value
}

type binaryExpression struct {
	left     expression
	operator string
	right    expression
}

func (e binaryExpression) expression() string {
	return "(" + e.left.expression() + " " + e.operator + " " + e.right.expression() + ")"
}

type parser struct {
	tokens []token
	offset int
}

func (p *parser) parseExpression() (expression, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (expression, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenOr {
		p.take()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = binaryExpression{left: left, operator: "OR", right: right}
	}
	return left, nil
}

func (p *parser) parseNot() (expression, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenNot {
		p.take()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = binaryExpression{left: left, operator: "NOT", right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (expression, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for startsPrimary(p.peek().kind) {
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = binaryExpression{left: left, operator: "AND", right: right}
	}
	return left, nil
}

func (p *parser) parsePrimary() (expression, error) {
	token := p.take()
	switch token.kind {
	case tokenTerm, tokenPhrase:
		return textExpression{text: token.text, prefix: token.prefix}, nil
	case tokenLeftParenthesis:
		node, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if p.take().kind != tokenRightParenthesis {
			return nil, invalidQuery("group is not closed")
		}
		return node, nil
	case tokenRightParenthesis:
		return nil, invalidQuery("unexpected closing parenthesis")
	case tokenOr, tokenNot:
		return nil, invalidQuery("operator %q has no left operand", token.text)
	default:
		return nil, invalidQuery("expression is incomplete")
	}
}

func (p *parser) peek() token {
	return p.tokens[p.offset]
}

func (p *parser) take() token {
	token := p.peek()
	if token.kind != tokenEnd {
		p.offset++
	}
	return token
}

func startsPrimary(kind tokenKind) bool {
	return kind == tokenTerm || kind == tokenPhrase || kind == tokenLeftParenthesis
}

func quoteText(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func invalidQuery(format string, arguments ...any) error {
	return fmt.Errorf("invalid search query: "+format, arguments...)
}
