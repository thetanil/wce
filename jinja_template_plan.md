# Jinja Template Implementation Plan

## Overview

This document outlines the strategy for implementing Jinja2-style templating in WCE using pure Starlark. This replaces the original Phase 7 plan which incorrectly assumed Go's `html/template` package would be used.

## Clarification of Requirements

### What We're NOT Doing
- ❌ Go's `html/template` or `text/template` packages
- ❌ Pre-rendering templates on save/commit
- ❌ Commit hooks for template rendering
- ❌ `_wce_page_cache` table for pre-rendered content
- ❌ Python's Jinja2 library (not available in Go/Starlark)

### What We ARE Doing
- ✅ Jinja2-style template syntax (variables, filters, control flow, inheritance)
- ✅ Templates stored as documents in `_wce_documents`
- ✅ **On-demand rendering** when a page is requested
- ✅ Pure Starlark implementation of Jinja parser and renderer
- ✅ Templates can be modified at runtime - changes take effect immediately on next request
- ✅ Auto-escaping for HTML security

## Jinja2 Syntax to Support

### Phase 1: Core Syntax (Minimum Viable)

```jinja2
{# Comments - ignored in output #}

{{ variable }}                    {# Variable output #}
{{ user.name }}                   {# Attribute access #}
{{ items[0] }}                    {# Subscript access #}
{{ value|upper }}                 {# Filters #}
{{ name|default("Guest") }}       {# Filter with argument #}

{% if condition %}                {# Conditionals #}
  content
{% elif other %}
  other content
{% else %}
  default content
{% endif %}

{% for item in items %}           {# Loops #}
  {{ loop.index }}: {{ item }}
{% endfor %}

{% set variable = value %}        {# Variable assignment #}
```

### Phase 2: Template Composition (Important)

```jinja2
{# Base template: base.html #}
<!DOCTYPE html>
<html>
<head>
  <title>{% block title %}Default Title{% endblock %}</title>
</head>
<body>
  {% block content %}{% endblock %}
</body>
</html>

{# Child template: page.html #}
{% extends "base.html" %}

{% block title %}My Page{% endblock %}

{% block content %}
  <h1>Welcome</h1>
  {% include "header.html" %}
{% endblock %}
```

### Phase 3: Advanced Features (Nice-to-Have)

```jinja2
{% macro render_user(user) %}     {# Reusable macros #}
  <div class="user">{{ user.name }}</div>
{% endmacro %}

{{ render_user(current_user) }}

{%- if condition -%}              {# Whitespace control #}
  content
{%- endif -%}

{% import 'forms.html' as forms %}
{{ forms.input('username') }}
```

## Architecture: Jinja in Starlark

### Component Overview

```
┌─────────────────────────────────────────────────────────┐
│  HTTP Request: GET /{cenvID}/pages/home                 │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│  Load Template Document from _wce_documents             │
│  - Document ID: "templates/pages/home.html"             │
│  - Content: Jinja2 template source                      │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│  Starlark Jinja Renderer                                │
│  ┌───────────────────────────────────────────┐          │
│  │ 1. Lexer/Tokenizer                        │          │
│  │    - Parse {{ }}, {% %}, {# #}            │          │
│  │    - Identify tokens: VAR, STMT, COMMENT  │          │
│  └───────────────────┬───────────────────────┘          │
│                      │                                   │
│  ┌───────────────────▼───────────────────────┐          │
│  │ 2. Parser                                 │          │
│  │    - Build AST from tokens                │          │
│  │    - Handle nesting, blocks, inheritance  │          │
│  └───────────────────┬───────────────────────┘          │
│                      │                                   │
│  ┌───────────────────▼───────────────────────┐          │
│  │ 3. Renderer                               │          │
│  │    - Walk AST with context data           │          │
│  │    - Apply filters, execute loops/ifs     │          │
│  │    - Auto-escape HTML                     │          │
│  │    - Handle template inheritance          │          │
│  └───────────────────┬───────────────────────┘          │
└────────────────────┬─┴────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│  Rendered HTML returned to client                       │
└─────────────────────────────────────────────────────────┘
```

### File Structure

```
internal/starlark/
├── jinja.star           # Starlark library for Jinja rendering
│   ├── lexer()          # Tokenize template
│   ├── parse()          # Build AST
│   ├── render()         # Render template with context
│   └── filters          # Built-in filters (upper, lower, etc.)
└── jinja_test.go        # Go tests that execute Starlark tests

internal/server/
├── templates.go         # HTTP handlers for template rendering
│   ├── handleRenderPage()
│   └── handlePreviewTemplate()
```

## Implementation Strategy

### Step 1: Starlark Jinja Library (Core)

Create `internal/starlark/jinja.star` - a pure Starlark implementation:

```python
# jinja.star - Jinja2 template engine in Starlark

def tokenize(template):
    """
    Lexer: Convert template string into tokens.
    Returns: list of (type, value, position) tuples

    Token types:
    - TEXT: Plain text content
    - VAR_BEGIN: {{
    - VAR_END: }}
    - STMT_BEGIN: {%
    - STMT_END: %}
    - COMMENT_BEGIN: {#
    - COMMENT_END: #}
    - IDENTIFIER: variable/keyword name
    - FILTER: |
    - DOT: .
    - LBRACKET: [
    - RBRACKET: ]
    """
    tokens = []
    pos = 0
    # ... tokenization logic ...
    return tokens

def parse(tokens):
    """
    Parser: Build Abstract Syntax Tree from tokens.
    Returns: AST (nested dict/list structure)

    AST node types:
    - {"type": "text", "value": "..."}
    - {"type": "var", "expr": {...}, "filters": [...]}
    - {"type": "if", "condition": {...}, "body": [...], "elif": [...], "else": [...]}
    - {"type": "for", "var": "item", "iterable": {...}, "body": [...]}
    - {"type": "block", "name": "...", "body": [...]}
    - {"type": "extends", "parent": "..."}
    - {"type": "include", "template": "..."}
    """
    ast = []
    # ... parsing logic ...
    return ast

def render(template, context, template_loader=None):
    """
    Main entry point: Render a Jinja template with context data.

    Args:
        template: Template string or AST
        context: Dict of variables to use in template
        template_loader: Function to load other templates (for extends/include)

    Returns: Rendered HTML string
    """
    if type(template) == "string":
        tokens = tokenize(template)
        ast = parse(tokens)
    else:
        ast = template

    # Handle template inheritance (extends)
    if ast and ast[0].get("type") == "extends":
        parent_name = ast[0]["parent"]
        parent_template = template_loader(parent_name)
        # Merge blocks from child into parent
        # ... inheritance logic ...

    output = []
    for node in ast:
        output.append(render_node(node, context, template_loader))

    return "".join(output)

def render_node(node, context, template_loader):
    """Render a single AST node."""
    node_type = node.get("type")

    if node_type == "text":
        return node["value"]

    elif node_type == "var":
        value = eval_expr(node["expr"], context)
        # Apply filters
        for filter_name, filter_args in node.get("filters", []):
            value = apply_filter(filter_name, value, filter_args, context)
        # Auto-escape HTML
        return html_escape(str(value))

    elif node_type == "if":
        if eval_expr(node["condition"], context):
            return render(node["body"], context, template_loader)
        for elif_clause in node.get("elif", []):
            if eval_expr(elif_clause["condition"], context):
                return render(elif_clause["body"], context, template_loader)
        if "else" in node:
            return render(node["else"], context, template_loader)
        return ""

    elif node_type == "for":
        items = eval_expr(node["iterable"], context)
        output = []
        for i, item in enumerate(items):
            loop_context = dict(context)
            loop_context[node["var"]] = item
            loop_context["loop"] = {
                "index": i + 1,
                "index0": i,
                "first": i == 0,
                "last": i == len(items) - 1,
                "length": len(items)
            }
            output.append(render(node["body"], loop_context, template_loader))
        return "".join(output)

    # ... other node types ...

    return ""

def eval_expr(expr, context):
    """Evaluate an expression (variable access, literals, operators)."""
    if expr["type"] == "identifier":
        return context.get(expr["name"], "")
    elif expr["type"] == "literal":
        return expr["value"]
    elif expr["type"] == "getattr":
        obj = eval_expr(expr["obj"], context)
        return getattr(obj, expr["attr"], "")
    elif expr["type"] == "getitem":
        obj = eval_expr(expr["obj"], context)
        key = eval_expr(expr["key"], context)
        return obj.get(key, "") if type(obj) == "dict" else obj[key]
    # ... more expression types ...

def apply_filter(name, value, args, context):
    """Apply a built-in filter."""
    if name == "upper":
        return value.upper()
    elif name == "lower":
        return value.lower()
    elif name == "default":
        return value if value else eval_expr(args[0], context)
    elif name == "length":
        return len(value)
    elif name == "join":
        sep = eval_expr(args[0], context) if args else ""
        return sep.join([str(x) for x in value])
    # ... more filters ...

    return value

def html_escape(text):
    """Escape HTML special characters."""
    text = str(text)
    text = text.replace("&", "&amp;")
    text = text.replace("<", "&lt;")
    text = text.replace(">", "&gt;")
    text = text.replace('"', "&quot;")
    text = text.replace("'", "&#x27;")
    return text
```

### Step 2: Go Wrapper for Jinja

Create `internal/template/template.go` (Go code):

```go
package template

import (
    "context"
    "fmt"

    "go.starlark.net/starlark"
    "github.com/thetanil/wce/internal/starlark"
)

// RenderTemplate renders a Jinja template using Starlark
func RenderTemplate(ctx context.Context, templateSource string, context map[string]interface{}) (string, error) {
    // Load the jinja.star library
    jinjaLib, err := loadJinjaLibrary()
    if err != nil {
        return "", err
    }

    // Convert Go context to Starlark dict
    starlarkContext := starlark_pkg.GoToStarlark(context)

    // Call render function
    thread := &starlark.Thread{Name: "jinja-render"}
    renderFunc := jinjaLib["render"]

    args := starlark.Tuple{
        starlark.String(templateSource),
        starlarkContext,
    }

    result, err := starlark.Call(thread, renderFunc, args, nil)
    if err != nil {
        return "", fmt.Errorf("template render error: %w", err)
    }

    // Convert result back to Go string
    return result.(starlark.String).GoString(), nil
}
```

### Step 3: Template Storage

Templates are stored as regular documents in `_wce_documents`:

```sql
INSERT INTO _wce_documents (
    id,
    content,
    content_type,
    created_by,
    modified_by
) VALUES (
    'templates/pages/home.html',
    '{% extends "templates/base.html" %}{% block content %}<h1>{{ title }}</h1>{% endblock %}',
    'text/html+jinja',
    'user-id',
    'user-id'
);
```

Document ID conventions:
- `templates/base.html` - Base templates
- `templates/pages/home.html` - Page templates
- `templates/components/header.html` - Reusable components

### Step 4: HTTP Endpoints

```go
// GET /{cenvID}/pages/{path...}
// Renders a template and returns HTML
func (s *Server) handleRenderPage(w http.ResponseWriter, r *http.Request) {
    cenvID := r.PathValue("cenvID")
    path := r.PathValue("path")

    // Load template from documents
    db := s.cenvManager.GetConnection(cenvID)
    templateID := "templates/pages/" + path + ".html"

    doc, err := document.GetDocument(db, templateID)
    if err != nil {
        http.Error(w, "Template not found", 404)
        return
    }

    // Prepare context data
    context := map[string]interface{}{
        "request": map[string]interface{}{
            "path": r.URL.Path,
            "method": r.Method,
        },
        // Add more context from query params, session, etc.
    }

    // Template loader for extends/include
    templateLoader := func(name string) (string, error) {
        doc, err := document.GetDocument(db, name)
        if err != nil {
            return "", err
        }
        return doc.Content, nil
    }

    // Render template
    html, err := template.RenderTemplate(r.Context(), doc.Content, context, templateLoader)
    if err != nil {
        http.Error(w, fmt.Sprintf("Render error: %v", err), 500)
        return
    }

    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Write([]byte(html))
}
```

## Testing Strategy

### Unit Tests (Starlark)

Create `internal/starlark/jinja_test.star`:

```python
# Test basic variable output
def test_variable_output():
    template = "Hello {{ name }}!"
    context = {"name": "World"}
    result = render(template, context)
    assert result == "Hello World!", "Expected 'Hello World!', got: " + result

# Test filters
def test_filter_upper():
    template = "{{ name|upper }}"
    context = {"name": "alice"}
    result = render(template, context)
    assert result == "ALICE"

# Test for loop
def test_for_loop():
    template = "{% for x in items %}{{ x }}{% endfor %}"
    context = {"items": [1, 2, 3]}
    result = render(template, context)
    assert result == "123"

# Test if statement
def test_if_statement():
    template = "{% if show %}yes{% else %}no{% endif %}"
    assert render(template, {"show": True}) == "yes"
    assert render(template, {"show": False}) == "no"

# Test template inheritance
def test_extends():
    base = "{% block content %}default{% endblock %}"
    child = "{% extends 'base' %}{% block content %}override{% endblock %}"

    loader = lambda name: base if name == "base" else ""
    result = render(child, {}, loader)
    assert result == "override"

# Test HTML escaping
def test_html_escape():
    template = "{{ html }}"
    context = {"html": "<script>alert('xss')</script>"}
    result = render(template, context)
    assert "&lt;script&gt;" in result
```

### Integration Tests (Go)

```go
func TestJinjaIntegration(t *testing.T) {
    // Create cenv with template documents
    // Render via HTTP endpoint
    // Verify output
}
```

## Modified Phase 7 Plan

### 7.1 Jinja Starlark Implementation ✅ **NEW**
- [x] Create `internal/starlark/jinja.star`
- [x] Implement lexer/tokenizer
- [x] Implement parser (AST builder)
- [x] Implement renderer
- [x] Implement core filters (upper, lower, default, length, join)
- [x] Implement control structures (if, for, set)
- [x] Implement template inheritance (extends, block)
- [x] Implement includes
- [x] Auto-escape HTML
- [x] Write comprehensive Starlark tests

### 7.2 Go Template Wrapper ✅ **NEW**
- [x] Create `internal/template/template.go`
- [x] `RenderTemplate()` function that calls Starlark jinja
- [x] Template loader function for extends/include
- [x] Error handling and reporting
- [x] Context conversion (Go ↔ Starlark)

### 7.3 Template Storage **MODIFIED**
- [x] Store templates as documents in `_wce_documents`
- [x] Use content_type: `text/html+jinja`
- [x] Document ID convention: `templates/{category}/{name}.html`
- [x] Use document tags: `template`, `page`, `component`
- [x] Leverage existing document versioning
- ❌ ~~No separate cache table~~ (removed)
- ❌ ~~No pre-rendering~~ (removed)

### 7.4 Page Rendering Endpoint ✅ **NEW**
- [x] Endpoint: `GET /{cenvID}/pages/{path...}`
- [x] Load template from documents
- [x] Render on-demand with Jinja
- [x] Return HTML with proper headers
- [x] Support template loader for extends/include
- [x] Pass request context to template
- [x] Handle render errors gracefully

### 7.5 Template Management **MODIFIED**
- [x] Templates are regular documents (use existing document API)
- [x] `GET/POST/PUT/DELETE /{cenvID}/documents` already works
- [x] Preview endpoint: `POST /{cenvID}/templates/preview` - render without saving
- [x] List templates: filter documents by content_type or tag

**REMOVED** from Phase 7:
- ❌ 7.3 Page Cache System (completely removed)
- ❌ 7.4 Commit Hook for Auto-Rendering (completely removed)
- ❌ 7.6 Markdown Rendering (defer to later or separate phase)

## Workflow Example

### Creating and Using a Template

```bash
# 1. Create a base template
POST /{cenvID}/documents
{
  "id": "templates/base.html",
  "content": "<!DOCTYPE html>\n<html>\n<head><title>{% block title %}Default{% endblock %}</title></head>\n<body>\n{% block content %}{% endblock %}\n</body>\n</html>",
  "content_type": "text/html+jinja",
  "tags": ["template", "base"]
}

# 2. Create a page template
POST /{cenvID}/documents
{
  "id": "templates/pages/home.html",
  "content": "{% extends 'templates/base.html' %}\n{% block title %}Home{% endblock %}\n{% block content %}<h1>Welcome {{ user.name }}!</h1>{% endblock %}",
  "content_type": "text/html+jinja",
  "tags": ["template", "page"]
}

# 3. Render the page
GET /{cenvID}/pages/home
# Returns fully rendered HTML

# 4. Update the template
PUT /{cenvID}/documents/templates/pages/home.html
{
  "content": "{% extends 'templates/base.html' %}\n{% block content %}<h1>Updated: {{ user.name }}!</h1>{% endblock %}"
}

# 5. Next request gets the updated version automatically
GET /{cenvID}/pages/home
# Returns HTML with updated content
```

## Migration from Current Plan

### Changes to IMPL.md

1. Update Phase 7 title to "Jinja Template System"
2. Remove all references to Go's `html/template`
3. Remove Phase 7.3 (Page Cache)
4. Remove Phase 7.4 (Commit Hooks)
5. Add new sections for Jinja Starlark implementation
6. Emphasize on-demand rendering

### Changes to CLAUDE.md & README.md

1. Change "Template System" description from "pre-rendered on commit" to "on-demand Jinja rendering"
2. Update architecture diagram to remove cache/commit hook flow
3. Add note about Jinja2-compatible syntax

## Implementation Timeline

### Iteration 1: Proof of Concept (1-2 days)
- Basic lexer for `{{ }}` and `{% %}`
- Simple variable output
- Basic if/for statements
- No inheritance yet
- Test with simple templates

### Iteration 2: Core Features (2-3 days)
- Complete parser for all syntax
- Template inheritance (extends/block)
- Includes
- All basic filters
- Loop variables (loop.index, etc.)
- Full test coverage

### Iteration 3: Integration (1-2 days)
- Go wrapper
- HTTP endpoints
- Template loader
- Document integration
- End-to-end tests

### Iteration 4: Advanced Features (1-2 days)
- Macros
- Whitespace control
- Custom filters
- Error reporting improvements
- Performance optimization

## References

- Jinja2 Documentation: https://jinja.palletsprojects.com/
- Jinja2 Template Designer Documentation: https://jinja.palletsprojects.com/en/3.1.x/templates/
- Starlark Language Spec: https://github.com/bazelbuild/starlark/blob/master/spec.md
- Starlark Go Implementation: https://pkg.go.dev/go.starlark.net/starlark

## Security Considerations

1. **Auto-escaping**: All variable output is HTML-escaped by default
2. **No code execution**: Templates cannot execute arbitrary code (no `eval`, `exec`)
3. **Sandboxed**: Starlark provides isolation from filesystem/network
4. **Template isolation**: Templates can only access provided context, not system resources
5. **Permission checks**: Template rendering respects user permissions for database queries

## Future Enhancements

- Template compilation/caching (parse once, render many times)
- Async template rendering
- Template fragments/AJAX partials
- Custom filter registration via Starlark
- Template debugging tools
- Performance profiling
