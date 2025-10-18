// Package template provides Jinja2-style template rendering using Starlark.
package template

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.starlark.net/starlark"
)

var jinjaStarlarkSource string

func init() {
	// Load jinja_simple.star from the same package
	// In production, this should be embedded or bundled
	dir, err := os.Getwd()
	if err != nil {
		jinjaStarlarkSource = fallbackSource()
		return
	}

	// Try to find jinja_iterative.star relative to current directory
	paths := []string{
		filepath.Join(dir, "internal/starlark/jinja_iterative.star"),
		filepath.Join(dir, "../starlark/jinja_iterative.star"),
		"internal/starlark/jinja_iterative.star",
	}

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err == nil {
			jinjaStarlarkSource = string(content)
			return
		}
	}

	// If we can't find it, use a minimal stub
	jinjaStarlarkSource = fallbackSource()
}

func fallbackSource() string {
	return `
def render(template, context, template_loader=None):
    return "Error: jinja_iterative.star not loaded properly"
`
}

// TemplateLoader is a function that loads template content by name/ID.
// Used for template inheritance (extends) and includes.
type TemplateLoader func(name string) (string, error)

// RenderContext holds all the data needed for template rendering
type RenderContext struct {
	Variables map[string]interface{} // Template variables
	Loader    TemplateLoader          // Template loader for extends/include
}

// RenderTemplate renders a Jinja2-style template using Go parser + Starlark execution.
//
// Args:
//   - ctx: Go context for cancellation/timeout
//   - templateSource: The Jinja template source code
//   - renderCtx: Rendering context with variables and loader
//
// Returns: Rendered HTML string or error
func RenderTemplate(ctx context.Context, templateSource string, renderCtx *RenderContext) (string, error) {
	// Check for timeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout := time.Until(deadline)
		if timeout <= 0 {
			return "", fmt.Errorf("template render context already expired")
		}
	}

	// Parse the template into AST (done in Go, no recursion issues!)
	nodes, err := ParseTemplate(templateSource)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	// Handle template inheritance (extends)
	nodes, err = handleInheritance(nodes, renderCtx.Loader)
	if err != nil {
		return "", fmt.Errorf("template inheritance error: %w", err)
	}

	// Render the AST (uses iteration, not recursion)
	return RenderAST(ctx, nodes, renderCtx.Variables, renderCtx.Loader)
}

func handleInheritance(nodes []Node, loader TemplateLoader) ([]Node, error) {
	// Check if first node is {% extends %}
	if len(nodes) > 0 && nodes[0].Type == NodeExtends {
		if loader == nil {
			return nil, fmt.Errorf("template loader required for extends")
		}

		// Load parent template
		parentSource, err := loader(nodes[0].Content)
		if err != nil {
			return nil, fmt.Errorf("failed to load parent template %s: %w", nodes[0].Content, err)
		}

		// Parse parent
		parentNodes, err := ParseTemplate(parentSource)
		if err != nil {
			return nil, fmt.Errorf("failed to parse parent template: %w", err)
		}

		// Extract blocks from child
		childBlocks := make(map[string][]Node)
		for _, node := range nodes {
			if node.Type == NodeBlock {
				childBlocks[node.Content] = node.Body
			}
		}

		// Merge blocks into parent
		mergedNodes := mergeBlocks(parentNodes, childBlocks)
		return mergedNodes, nil
	}

	return nodes, nil
}

func mergeBlocks(parentNodes []Node, childBlocks map[string][]Node) []Node {
	result := make([]Node, len(parentNodes))
	for i, node := range parentNodes {
		if node.Type == NodeBlock {
			// Check if child overrides this block
			if childBody, ok := childBlocks[node.Content]; ok {
				node.Body = childBody
			}
		}
		result[i] = node
	}
	return result
}

// loadJinjaLibrary loads and executes the jinja_iterative.star Starlark module
func loadJinjaLibrary(thread *starlark.Thread) (starlark.StringDict, error) {
	// Check if source is loaded
	if jinjaStarlarkSource == "" || len(jinjaStarlarkSource) < 100 {
		return nil, fmt.Errorf("jinja starlark source not loaded (len=%d)", len(jinjaStarlarkSource))
	}

	// Execute the jinja_iterative.star source
	// Note: ExecFile returns the globals, it doesn't modify the input dict
	globals, err := starlark.ExecFile(thread, "jinja_iterative.star", jinjaStarlarkSource, nil)
	if err != nil {
		return nil, fmt.Errorf("starlark execution error: %w", err)
	}

	return globals, nil
}

// makeStarlarkLoader wraps a Go TemplateLoader into a Starlark callable
func makeStarlarkLoader(loader TemplateLoader, ctx context.Context) starlark.Callable {
	return starlark.NewBuiltin("template_loader", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("template loading cancelled: %w", ctx.Err())
		default:
		}

		// Expect single string argument (template name)
		if len(args) != 1 {
			return nil, fmt.Errorf("template_loader expects 1 argument, got %d", len(args))
		}

		name, ok := args[0].(starlark.String)
		if !ok {
			return nil, fmt.Errorf("template_loader expects string argument, got %T", args[0])
		}

		// Load template
		content, err := loader(name.GoString())
		if err != nil {
			return nil, fmt.Errorf("failed to load template '%s': %w", name.GoString(), err)
		}

		return starlark.String(content), nil
	})
}

// goToStarlark converts a Go map[string]interface{} to a Starlark dict
func goToStarlark(data map[string]interface{}) *starlark.Dict {
	dict := starlark.NewDict(len(data))

	for key, value := range data {
		starlarkKey := starlark.String(key)
		starlarkValue := valueToStarlark(value)
		dict.SetKey(starlarkKey, starlarkValue)
	}

	return dict
}

// valueToStarlark converts a Go value to a Starlark value
func valueToStarlark(value interface{}) starlark.Value {
	if value == nil {
		return starlark.None
	}

	switch v := value.(type) {
	case string:
		return starlark.String(v)
	case int:
		return starlark.MakeInt(v)
	case int64:
		return starlark.MakeInt64(v)
	case float64:
		return starlark.Float(v)
	case bool:
		return starlark.Bool(v)
	case []interface{}:
		items := make([]starlark.Value, len(v))
		for i, item := range v {
			items[i] = valueToStarlark(item)
		}
		return starlark.NewList(items)
	case map[string]interface{}:
		dict := starlark.NewDict(len(v))
		for key, val := range v {
			dict.SetKey(starlark.String(key), valueToStarlark(val))
		}
		return dict
	default:
		// Fallback: convert to string
		return starlark.String(fmt.Sprintf("%v", v))
	}
}

// DocumentLoader creates a TemplateLoader that loads templates from the document store
func DocumentLoader(db *sql.DB) TemplateLoader {
	return func(name string) (string, error) {
		// Query _wce_documents for the template
		var content string
		err := db.QueryRow(`
			SELECT content
			FROM _wce_documents
			WHERE id = ?
		`, name).Scan(&content)

		if err == sql.ErrNoRows {
			return "", fmt.Errorf("template not found: %s", name)
		}
		if err != nil {
			return "", fmt.Errorf("database error loading template: %w", err)
		}

		return content, nil
	}
}

// RenderTemplateFromDB is a convenience function that renders a template loaded from the database
func RenderTemplateFromDB(ctx context.Context, db *sql.DB, templateID string, variables map[string]interface{}) (string, error) {
	// Load the template document
	var templateSource string
	err := db.QueryRow(`
		SELECT content
		FROM _wce_documents
		WHERE id = ?
	`, templateID).Scan(&templateSource)

	if err == sql.ErrNoRows {
		return "", fmt.Errorf("template not found: %s", templateID)
	}
	if err != nil {
		return "", fmt.Errorf("database error: %w", err)
	}

	// Create render context with document loader
	renderCtx := &RenderContext{
		Variables: variables,
		Loader:    DocumentLoader(db),
	}

	// Render the template
	return RenderTemplate(ctx, templateSource, renderCtx)
}
