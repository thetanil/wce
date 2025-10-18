package template

import (
	"context"
	"fmt"
	"html"
	"strings"

	"go.starlark.net/starlark"
)

// RenderAST renders a parsed template AST with the given context
func RenderAST(ctx context.Context, nodes []Node, context map[string]interface{}, loader TemplateLoader) (string, error) {
	// Create Starlark thread for expression evaluation
	thread := &starlark.Thread{Name: "template-render"}

	// Convert context to Starlark
	starlarkCtx := goToStarlark(context)

	var output strings.Builder

	for _, node := range nodes {
		rendered, err := renderNode(ctx, thread, node, starlarkCtx, context, loader)
		if err != nil {
			return "", err
		}
		output.WriteString(rendered)
	}

	return output.String(), nil
}

func renderNode(ctx context.Context, thread *starlark.Thread, node Node, starlarkCtx *starlark.Dict, context map[string]interface{}, loader TemplateLoader) (string, error) {
	switch node.Type {
	case NodeText:
		return node.Content, nil

	case NodeVariable:
		// Evaluate expression using Starlark
		value, err := evalExpression(thread, node.Expr, starlarkCtx)
		if err != nil {
			return "", fmt.Errorf("error evaluating %s: %w", node.Expr, err)
		}
		// Convert to Go value first, then to string
		goValue := starlarkToGo(value)
		strValue := fmt.Sprintf("%v", goValue)
		// Auto-escape HTML
		return html.EscapeString(strValue), nil

	case NodeFor:
		// Evaluate iterable
		items, err := evalExpression(thread, node.Iterable, starlarkCtx)
		if err != nil {
			return "", fmt.Errorf("error evaluating iterable %s: %w", node.Iterable, err)
		}

		// Convert to Go slice
		itemsList, ok := starlarkToGo(items).([]interface{})
		if !ok {
			// Try to convert other iterables
			if list, ok := items.(*starlark.List); ok {
				itemsList = make([]interface{}, list.Len())
				for i := 0; i < list.Len(); i++ {
					itemsList[i] = starlarkToGo(list.Index(i))
				}
			} else {
				return "", fmt.Errorf("for loop iterable is not a list")
			}
		}

		// Render loop
		var output strings.Builder
		for i, item := range itemsList {
			// Create loop context
			loopCtx := make(map[string]interface{})
			for k, v := range context {
				loopCtx[k] = v
			}
			loopCtx[node.Variable] = item
			loopCtx["loop"] = map[string]interface{}{
				"index":  i + 1,
				"index0": i,
				"first":  i == 0,
				"last":   i == len(itemsList)-1,
				"length": len(itemsList),
			}

			// Convert to Starlark context
			loopStarlarkCtx := goToStarlark(loopCtx)

			// Render body (no recursion - just iterate over body nodes)
			for _, bodyNode := range node.Body {
				rendered, err := renderNode(ctx, thread, bodyNode, loopStarlarkCtx, loopCtx, loader)
				if err != nil {
					return "", err
				}
				output.WriteString(rendered)
			}
		}
		return output.String(), nil

	case NodeIf:
		// Evaluate condition
		result, err := evalExpression(thread, node.Expr, starlarkCtx)
		if err != nil {
			return "", fmt.Errorf("error evaluating condition %s: %w", node.Expr, err)
		}

		// Check truthiness
		var output strings.Builder
		if isTruthy(result) {
			// Render if body
			for _, bodyNode := range node.Body {
				rendered, err := renderNode(ctx, thread, bodyNode, starlarkCtx, context, loader)
				if err != nil {
					return "", err
				}
				output.WriteString(rendered)
			}
		} else if len(node.ElseBody) > 0 {
			// Render else body
			for _, bodyNode := range node.ElseBody {
				rendered, err := renderNode(ctx, thread, bodyNode, starlarkCtx, context, loader)
				if err != nil {
					return "", err
				}
				output.WriteString(rendered)
			}
		}
		return output.String(), nil

	case NodeSet:
		// Evaluate expression and set variable
		value, err := evalExpression(thread, node.Expr, starlarkCtx)
		if err != nil {
			return "", fmt.Errorf("error evaluating %s: %w", node.Expr, err)
		}

		// Update both contexts
		context[node.Variable] = starlarkToGo(value)
		starlarkCtx.SetKey(starlark.String(node.Variable), value)
		return "", nil

	case NodeInclude:
		// Load and render included template
		if loader == nil {
			return "", fmt.Errorf("template loader required for include")
		}

		includedSource, err := loader(node.Content)
		if err != nil {
			return "", fmt.Errorf("failed to load template %s: %w", node.Content, err)
		}

		// Parse and render included template
		includedNodes, err := ParseTemplate(includedSource)
		if err != nil {
			return "", fmt.Errorf("failed to parse included template: %w", err)
		}

		return RenderAST(ctx, includedNodes, context, loader)

	case NodeBlock:
		// Render block body
		var output strings.Builder
		for _, bodyNode := range node.Body {
			rendered, err := renderNode(ctx, thread, bodyNode, starlarkCtx, context, loader)
			if err != nil {
				return "", err
			}
			output.WriteString(rendered)
		}
		return output.String(), nil

	case NodeExtends:
		// This should be handled at the top level
		return "", fmt.Errorf("extends node should not be rendered directly")

	default:
		return "", fmt.Errorf("unknown node type: %d", node.Type)
	}
}

// evalExpression evaluates a Jinja2 expression using Starlark
func evalExpression(thread *starlark.Thread, expr string, context *starlark.Dict) (starlark.Value, error) {
	// Handle filters: var|filter
	if strings.Contains(expr, "|") {
		parts := strings.SplitN(expr, "|", 2)
		baseExpr := strings.TrimSpace(parts[0])
		filterExpr := strings.TrimSpace(parts[1])

		// Evaluate base expression
		value, err := evalSimpleExpression(thread, baseExpr, context)
		if err != nil {
			return nil, err
		}

		// Apply filter
		return applyFilter(filterExpr, value)
	}

	return evalSimpleExpression(thread, expr, context)
}

func evalSimpleExpression(thread *starlark.Thread, expr string, context *starlark.Dict) (starlark.Value, error) {
	// String literal
	if strings.HasPrefix(expr, `"`) || strings.HasPrefix(expr, "'") {
		return starlark.String(strings.Trim(expr, `"'`)), nil
	}

	// Number literal
	if strings.IndexAny(expr, "0123456789") == 0 {
		// Try parsing as int
		if val, err := starlark.NumberToInt(starlark.MakeInt(0)); err == nil {
			_ = val
			// For simplicity, just use string lookup
		}
	}

	// Attribute access: obj.attr (can be nested: obj.attr.subattr)
	if strings.Contains(expr, ".") {
		parts := strings.Split(expr, ".")

		// Start with the first part
		obj, found, err := context.Get(starlark.String(parts[0]))
		if err != nil || !found || obj == nil {
			// DEBUG
			//fmt.Printf("DEBUG: Failed to find '%s' in context\n", parts[0])
			return starlark.String(""), nil
		}

		// Navigate through the remaining parts
		for i := 1; i < len(parts); i++ {
			if dict, ok := obj.(*starlark.Dict); ok {
				// Starlark dict.Get() requires exact key match
				// Try direct lookup first
				lookupKey := starlark.String(parts[i])
				val, found, _ := dict.Get(lookupKey)

				// If not found, iterate to find by string value comparison
				// (dict.Get might not work if keys were created differently)
				if !found || val == nil {
					for _, item := range dict.Items() {
						k := item[0]
						if kstr, ok := k.(starlark.String); ok {
							// Compare actual string values
							if string(kstr) == parts[i] {
								val = item[1]
								found = true
								break
							}
						}
					}

					if !found || val == nil {
						// Still not found
						return starlark.String(""), nil
					}
				}

				obj = val
			} else {
				// Not a dict, can't access attributes
				return starlark.String(""), nil
			}
		}

		return obj, nil
	}

	// Simple variable lookup
	val, found, err := context.Get(starlark.String(expr))
	if err != nil || !found || val == nil {
		return starlark.String(""), nil
	}

	return val, nil
}

func applyFilter(filterExpr string, value starlark.Value) (starlark.Value, error) {
	// Parse filter name and arguments
	filterName := filterExpr
	var args []string

	if strings.Contains(filterExpr, "(") {
		idx := strings.Index(filterExpr, "(")
		filterName = filterExpr[:idx]
		argsStr := strings.TrimSuffix(strings.TrimPrefix(filterExpr[idx:], "("), ")")
		if argsStr != "" {
			args = []string{strings.Trim(argsStr, `"'`)}
		}
	}

	// Convert value to Go value first, then to string
	goValue := starlarkToGo(value)
	strVal := fmt.Sprintf("%v", goValue)

	switch filterName {
	case "upper":
		return starlark.String(strings.ToUpper(strVal)), nil

	case "lower":
		return starlark.String(strings.ToLower(strVal)), nil

	case "title":
		return starlark.String(strings.Title(strVal)), nil

	case "capitalize":
		if len(strVal) > 0 {
			return starlark.String(strings.ToUpper(strVal[:1]) + strings.ToLower(strVal[1:])), nil
		}
		return starlark.String(strVal), nil

	case "default":
		if strVal == "" || strVal == "None" || strVal == "<nil>" {
			if len(args) > 0 {
				return starlark.String(args[0]), nil
			}
			return starlark.String(""), nil
		}
		return value, nil

	case "length":
		if list, ok := value.(*starlark.List); ok {
			return starlark.MakeInt(list.Len()), nil
		}
		if dict, ok := value.(*starlark.Dict); ok {
			return starlark.MakeInt(dict.Len()), nil
		}
		return starlark.MakeInt(len(strVal)), nil

	case "join":
		sep := ""
		if len(args) > 0 {
			sep = args[0]
		}
		if list, ok := value.(*starlark.List); ok {
			var parts []string
			for i := 0; i < list.Len(); i++ {
				goVal := starlarkToGo(list.Index(i))
				parts = append(parts, fmt.Sprintf("%v", goVal))
			}
			return starlark.String(strings.Join(parts, sep)), nil
		}
		return value, nil

	default:
		return value, nil
	}
}

func isTruthy(value starlark.Value) bool {
	if value == nil || value == starlark.None {
		return false
	}

	if b, ok := value.(starlark.Bool); ok {
		return bool(b)
	}

	if s, ok := value.(starlark.String); ok {
		return string(s) != ""
	}

	if i, ok := value.(starlark.Int); ok {
		return i.Sign() != 0
	}

	if list, ok := value.(*starlark.List); ok {
		return list.Len() > 0
	}

	if dict, ok := value.(*starlark.Dict); ok {
		return dict.Len() > 0
	}

	return true
}

func starlarkToGo(value starlark.Value) interface{} {
	if value == nil || value == starlark.None {
		return nil
	}

	switch v := value.(type) {
	case starlark.Bool:
		return bool(v)
	case starlark.String:
		return string(v)
	case starlark.Int:
		i, _ := v.Int64()
		return int(i)
	case starlark.Float:
		return float64(v)
	case *starlark.List:
		result := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			result[i] = starlarkToGo(v.Index(i))
		}
		return result
	case *starlark.Dict:
		result := make(map[string]interface{})
		for _, item := range v.Items() {
			key := item[0].String()
			result[key] = starlarkToGo(item[1])
		}
		return result
	default:
		return v.String()
	}
}
