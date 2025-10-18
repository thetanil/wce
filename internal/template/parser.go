package template

import (
	"fmt"
	"strings"
)

// Simple template parser that handles Jinja2 syntax
// This runs in Go (not Starlark) to avoid recursion issues

type NodeType int

const (
	NodeText NodeType = iota
	NodeVariable
	NodeFor
	NodeIf
	NodeSet
	NodeInclude
	NodeBlock
	NodeExtends
)

type Node struct {
	Type     NodeType
	Content  string            // For text nodes
	Expr     string            // For variable/condition expressions
	Variable string            // For 'for' and 'set' nodes
	Iterable string            // For 'for' nodes
	Body     []Node            // For control structures
	ElseBody []Node            // For if/else
	Blocks   map[string][]Node // For extends/blocks
}

// ParseTemplate parses a Jinja2 template into an AST
func ParseTemplate(source string) ([]Node, error) {
	return parseNodes(source, 0)
}

func parseNodes(source string, depth int) ([]Node, error) {
	if depth > 50 {
		return nil, fmt.Errorf("template nesting too deep")
	}

	nodes := []Node{}
	remaining := source

	for len(remaining) > 0 {
		// Find next tag
		commentIdx := strings.Index(remaining, "{#")
		varIdx := strings.Index(remaining, "{{")
		stmtIdx := strings.Index(remaining, "{%")

		// Find the nearest tag
		nextIdx := len(remaining)
		tagType := ""

		if commentIdx >= 0 && commentIdx < nextIdx {
			nextIdx = commentIdx
			tagType = "comment"
		}
		if varIdx >= 0 && varIdx < nextIdx {
			nextIdx = varIdx
			tagType = "var"
		}
		if stmtIdx >= 0 && stmtIdx < nextIdx {
			nextIdx = stmtIdx
			tagType = "stmt"
		}

		// Add text before tag
		if nextIdx > 0 {
			nodes = append(nodes, Node{
				Type:    NodeText,
				Content: remaining[:nextIdx],
			})
			remaining = remaining[nextIdx:]
		}

		if tagType == "" {
			break // No more tags
		}

		// Process the tag
		switch tagType {
		case "comment":
			// Skip comment
			endIdx := strings.Index(remaining, "#}")
			if endIdx < 0 {
				return nil, fmt.Errorf("unclosed comment")
			}
			remaining = remaining[endIdx+2:]

		case "var":
			// Variable tag {{ expr }}
			endIdx := strings.Index(remaining, "}}")
			if endIdx < 0 {
				return nil, fmt.Errorf("unclosed variable tag")
			}
			expr := strings.TrimSpace(remaining[2:endIdx])
			nodes = append(nodes, Node{
				Type: NodeVariable,
				Expr: expr,
			})
			remaining = remaining[endIdx+2:]

		case "stmt":
			// Statement tag {% ... %}
			endIdx := strings.Index(remaining, "%}")
			if endIdx < 0 {
				return nil, fmt.Errorf("unclosed statement tag")
			}
			stmt := strings.TrimSpace(remaining[2:endIdx])
			remaining = remaining[endIdx+2:]

			// Parse the statement
			node, newRemaining, err := parseStatement(stmt, remaining, depth)
			if err != nil {
				return nil, err
			}
			if node != nil {
				nodes = append(nodes, *node)
			}
			remaining = newRemaining
		}
	}

	return nodes, nil
}

func parseStatement(stmt, remaining string, depth int) (*Node, string, error) {
	parts := strings.Fields(stmt)
	if len(parts) == 0 {
		return nil, remaining, nil
	}

	keyword := parts[0]

	switch keyword {
	case "for":
		// {% for item in items %}
		return parseFor(stmt, remaining, depth)

	case "if":
		// {% if condition %}
		return parseIf(stmt, remaining, depth)

	case "set":
		// {% set var = value %}
		return parseSet(stmt), remaining, nil

	case "include":
		// {% include "template" %}
		templateName := extractQuoted(stmt[8:])
		return &Node{
			Type:    NodeInclude,
			Content: templateName,
		}, remaining, nil

	case "extends":
		// {% extends "parent" %}
		parentName := extractQuoted(stmt[8:])
		return &Node{
			Type:    NodeExtends,
			Content: parentName,
		}, remaining, nil

	case "block":
		// {% block name %}
		blockName := strings.TrimSpace(stmt[6:])
		return parseBlock(blockName, remaining, depth)

	case "endfor", "endif", "endblock":
		// These are handled by their opening tags
		return nil, remaining, nil

	default:
		return nil, remaining, fmt.Errorf("unknown statement: %s", keyword)
	}
}

func parseFor(stmt, remaining string, depth int) (*Node, string, error) {
	// Parse: for var in iterable
	parts := strings.Split(stmt, " in ")
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid for statement: %s", stmt)
	}

	varName := strings.TrimSpace(strings.TrimPrefix(parts[0], "for"))
	iterable := strings.TrimSpace(parts[1])

	// Find matching endfor
	body, newRemaining, err := findBlockEnd(remaining, "{% for ", "{% endfor %}")
	if err != nil {
		return nil, "", err
	}

	// Parse body
	bodyNodes, err := parseNodes(body, depth+1)
	if err != nil {
		return nil, "", err
	}

	return &Node{
		Type:     NodeFor,
		Variable: varName,
		Iterable: iterable,
		Body:     bodyNodes,
	}, newRemaining, nil
}

func parseIf(stmt, remaining string, depth int) (*Node, string, error) {
	// Parse: if condition
	condition := strings.TrimSpace(strings.TrimPrefix(stmt, "if"))

	// Find matching endif and handle else
	body, elseBody, newRemaining, err := findIfBlockEnd(remaining)
	if err != nil {
		return nil, "", err
	}

	// Parse body
	bodyNodes, err := parseNodes(body, depth+1)
	if err != nil {
		return nil, "", err
	}

	node := &Node{
		Type: NodeIf,
		Expr: condition,
		Body: bodyNodes,
	}

	// Parse else body if present
	if elseBody != "" {
		elseNodes, err := parseNodes(elseBody, depth+1)
		if err != nil {
			return nil, "", err
		}
		node.ElseBody = elseNodes
	}

	return node, newRemaining, nil
}

func findIfBlockEnd(source string) (string, string, string, error) {
	depth := 1
	pos := 0
	elsePos := -1

	for pos < len(source) {
		// Look for {% if, {% else %}, {% endif %}
		if strings.HasPrefix(source[pos:], "{% if ") {
			depth++
			pos += 6
		} else if strings.HasPrefix(source[pos:], "{% endif %}") {
			depth--
			if depth == 0 {
				// Found matching endif
				var body, elseBody string
				if elsePos >= 0 {
					body = source[:elsePos]
					elseBody = source[elsePos+10 : pos] // Skip "{% else %}"
				} else {
					body = source[:pos]
				}
				remaining := source[pos+11:]
				return body, elseBody, remaining, nil
			}
			pos += 11
		} else if strings.HasPrefix(source[pos:], "{% else %}") && depth == 1 && elsePos < 0 {
			// Found else at our level
			elsePos = pos
			pos += 10
		} else {
			pos++
		}
	}

	return "", "", "", fmt.Errorf("unclosed if block")
}

func parseSet(stmt string) *Node {
	// Parse: set var = value
	parts := strings.SplitN(stmt, "=", 2)
	if len(parts) != 2 {
		return nil
	}

	varName := strings.TrimSpace(strings.TrimPrefix(parts[0], "set"))
	expr := strings.TrimSpace(parts[1])

	return &Node{
		Type:     NodeSet,
		Variable: varName,
		Expr:     expr,
	}
}

func parseBlock(blockName, remaining string, depth int) (*Node, string, error) {
	// Find matching endblock
	body, newRemaining, err := findBlockEnd(remaining, "{% block ", "{% endblock %}")
	if err != nil {
		return nil, "", err
	}

	// Parse body
	bodyNodes, err := parseNodes(body, depth+1)
	if err != nil {
		return nil, "", err
	}

	node := &Node{
		Type:    NodeBlock,
		Content: blockName,
		Body:    bodyNodes,
	}

	if node.Blocks == nil {
		node.Blocks = make(map[string][]Node)
	}
	node.Blocks[blockName] = bodyNodes

	return node, newRemaining, nil
}

func findBlockEnd(source, openTag, closeTag string) (string, string, error) {
	depth := 1
	pos := 0

	for pos < len(source) {
		openIdx := strings.Index(source[pos:], openTag)
		closeIdx := strings.Index(source[pos:], closeTag)

		if closeIdx < 0 {
			return "", "", fmt.Errorf("unclosed block")
		}

		if openIdx >= 0 && openIdx < closeIdx {
			// Found nested open tag
			depth++
			pos += openIdx + len(openTag)
		} else {
			// Found close tag
			depth--
			if depth == 0 {
				body := source[:pos+closeIdx]
				remaining := source[pos+closeIdx+len(closeTag):]
				return body, remaining, nil
			}
			pos += closeIdx + len(closeTag)
		}
	}

	return "", "", fmt.Errorf("unclosed block")
}

func extractQuoted(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	return s
}
