# jinja_simple.star - Simplified Jinja2-compatible template engine for Starlark
#
# This is a simplified version that works within Starlark's constraints:
# - No while loops (Starlark doesn't support them)
# - Uses string operations and recursion
# - Supports core Jinja features: variables, filters, if, for, extends, blocks
#
# Note: This is a proof-of-concept. For production use, the lexer/parser
# should be implemented in Go and only the execution done in Starlark.

def render(template_source, context, template_loader=None):
    """
    Main entry point: Render a Jinja template with context data.

    Args:
        template_source: Template string
        context: Dict of variables
        template_loader: Function(name) -> template_source for extends/include

    Returns: Rendered HTML string
    """
    # Handle template inheritance (extends)
    if "{% extends" in template_source:
        return render_with_extends(template_source, context, template_loader)

    # Render the template
    return render_template(template_source, context, template_loader)

def render_with_extends(template_source, context, template_loader):
    """Handle template inheritance"""
    # Extract extends statement
    start = template_source.find("{% extends")
    if start == -1:
        return render_template(template_source, context, template_loader)

    end = template_source.find("%}", start)
    extends_stmt = template_source[start+10:end].strip()

    # Remove quotes
    parent_name = extends_stmt.strip().strip("'").strip('"')

    # Load parent template
    if not template_loader:
        fail("Template loader required for extends")

    parent_source = template_loader(parent_name)

    # Extract blocks from child
    child_blocks = extract_blocks(template_source)

    # Merge blocks into parent
    merged = merge_blocks(parent_source, child_blocks)

    return render_template(merged, context, template_loader)

def extract_blocks(template):
    """Extract all block definitions from a template"""
    blocks = {}
    remaining = template

    # Simple extraction using string operations
    for _ in range(20):  # Limit iterations
        start = remaining.find("{% block ")
        if start == -1:
            break

        # Find block name
        name_start = start + 9
        name_end = remaining.find("%}", name_start)
        block_name = remaining[name_start:name_end].strip()

        # Find endblock
        end_tag = "{% endblock %}"
        end = remaining.find(end_tag, name_end)
        if end == -1:
            break

        # Extract block content
        content_start = name_end + 2
        content = remaining[content_start:end]

        blocks[block_name] = content
        remaining = remaining[end + len(end_tag):]

    return blocks

def merge_blocks(parent, child_blocks):
    """Merge child blocks into parent template"""
    result = parent

    for block_name, content in child_blocks.items():
        # Find block in parent
        search = "{% block " + block_name + " %}"
        start = result.find(search)
        if start == -1:
            continue

        # Find endblock
        end_tag = "{% endblock %}"
        end = result.find(end_tag, start)
        if end == -1:
            continue

        # Replace block content
        before = result[:start + len(search)]
        after = result[end:]
        result = before + content + after

    return result

def render_template(template, context, template_loader):
    """Render a template (without extends)"""
    output = []
    remaining = template

    # Process template in chunks
    for _ in range(1000):  # Limit iterations to prevent infinite loops
        if not remaining:
            break

        # Check for {# comment #}
        comment_start = remaining.find("{#")
        var_start = remaining.find("{{")
        stmt_start = remaining.find("{%")

        # Find nearest tag
        next_tag = len(remaining)
        tag_type = None

        if comment_start != -1 and comment_start < next_tag:
            next_tag = comment_start
            tag_type = "comment"
        if var_start != -1 and var_start < next_tag:
            next_tag = var_start
            tag_type = "var"
        if stmt_start != -1 and stmt_start < next_tag:
            next_tag = stmt_start
            tag_type = "stmt"

        # Add text before tag
        if next_tag > 0:
            output.append(remaining[:next_tag])

        if tag_type == None:
            # No more tags
            break

        remaining = remaining[next_tag:]

        # Process tag
        if tag_type == "comment":
            # Skip comment
            end = remaining.find("#}")
            if end == -1:
                fail("Unclosed comment")
            remaining = remaining[end+2:]

        elif tag_type == "var":
            # Process variable
            end = remaining.find("}}")
            if end == -1:
                fail("Unclosed variable tag")

            expr = remaining[2:end].strip()
            value = eval_variable(expr, context)
            output.append(html_escape(str(value)))
            remaining = remaining[end+2:]

        elif tag_type == "stmt":
            # Process statement
            result_text, new_remaining = process_statement(remaining, context, template_loader)
            output.append(result_text)
            remaining = new_remaining

    return "".join(output)

def process_statement(template, context, template_loader):
    """Process a statement {% ... %}"""
    end = template.find("%}")
    if end == -1:
        fail("Unclosed statement tag")

    stmt = template[2:end].strip()
    remaining = template[end+2:]

    # Parse statement
    parts = stmt.split(" ", 1)
    keyword = parts[0]

    if keyword == "if":
        return process_if(stmt[3:].strip(), remaining, context, template_loader)
    elif keyword == "for":
        return process_for(stmt[4:].strip(), remaining, context, template_loader)
    elif keyword == "set":
        return process_set(stmt[4:].strip(), context, remaining)
    elif keyword == "include":
        return process_include(stmt[8:].strip(), context, template_loader, remaining)
    elif keyword == "block":
        # Skip block tags (handled by extends)
        block_end = remaining.find("{% endblock %}")
        if block_end != -1:
            content = remaining[:block_end]
            remaining = remaining[block_end+15:]
            rendered = render_template(content, context, template_loader)
            return rendered, remaining
        return "", remaining
    elif keyword in ["endif", "endfor", "endblock", "elif", "else"]:
        # These are handled by their parent statements
        return "", remaining

    return "", remaining

def process_if(condition, remaining, context, template_loader):
    """Process {% if condition %} ... {% endif %}"""
    # Find body
    body_start = 0
    depth = 1
    pos = 0

    # Find matching endif
    for i in range(len(remaining)):
        if remaining[i:i+7] == "{% if ":
            depth += 1
        elif remaining[i:i+11] == "{% endif %}":
            depth -= 1
            if depth == 0:
                pos = i
                break
        elif remaining[i:i+9] == "{% elif " and depth == 1:
            # TODO: Handle elif
            pass
        elif remaining[i:i+9] == "{% else %}" and depth == 1:
            # TODO: Handle else
            pass

    if depth != 0:
        fail("Unclosed if statement")

    body = remaining[:pos]
    after = remaining[pos+11:]

    # Evaluate condition
    if eval_condition(condition, context):
        rendered = render_template(body, context, template_loader)
        return rendered, after

    return "", after

def process_for(for_stmt, remaining, context, template_loader):
    """Process {% for item in items %} ... {% endfor %}"""
    # Parse: var in iterable
    parts = for_stmt.split(" in ", 1)
    if len(parts) != 2:
        fail("Invalid for statement")

    var_name = parts[0].strip()
    iterable_expr = parts[1].strip()

    # Find body
    depth = 1
    pos = 0

    for i in range(len(remaining)):
        if remaining[i:i+8] == "{% for ":
            depth += 1
        elif remaining[i:i+13] == "{% endfor %}":
            depth -= 1
            if depth == 0:
                pos = i
                break

    if depth != 0:
        fail("Unclosed for statement")

    body = remaining[:pos]
    after = remaining[pos+13:]

    # Evaluate iterable
    items = eval_variable(iterable_expr, context)
    if type(items) not in ["list", "tuple"]:
        return "", after

    # Render loop
    output = []
    for i, item in enumerate(items):
        loop_context = dict(context)
        loop_context[var_name] = item
        loop_context["loop"] = {
            "index": i + 1,
            "index0": i,
            "first": i == 0,
            "last": i == len(items) - 1,
            "length": len(items),
        }
        output.append(render_template(body, loop_context, template_loader))

    return "".join(output), after

def process_set(set_stmt, context, remaining):
    """Process {% set var = value %}"""
    parts = set_stmt.split("=", 1)
    if len(parts) != 2:
        fail("Invalid set statement")

    var_name = parts[0].strip()
    value_expr = parts[1].strip()

    value = eval_variable(value_expr, context)
    context[var_name] = value

    return "", remaining

def process_include(include_stmt, context, template_loader, remaining):
    """Process {% include "template" %}"""
    if not template_loader:
        fail("Template loader required for include")

    # Remove quotes
    template_name = include_stmt.strip().strip("'").strip('"')

    # Load and render template
    included_source = template_loader(template_name)
    rendered = render(included_source, context, template_loader)

    return rendered, remaining

def eval_variable(expr, context):
    """Evaluate a variable expression"""
    # Handle filters: var|filter
    if "|" in expr:
        parts = expr.split("|", 1)
        value = eval_simple_expr(parts[0].strip(), context)
        filter_expr = parts[1].strip()
        return apply_filter(filter_expr, value, context)

    return eval_simple_expr(expr, context)

def eval_simple_expr(expr, context):
    """Evaluate a simple expression (no filters)"""
    # String literal
    if expr.startswith('"') or expr.startswith("'"):
        return expr[1:-1]

    # Number literal
    if expr.isdigit():
        return int(expr)

    # Attribute access: obj.attr
    if "." in expr:
        parts = expr.split(".", 1)
        obj = context.get(parts[0], {})
        if type(obj) == "dict":
            return obj.get(parts[1], "")
        return ""

    # Subscript access: obj[key]
    if "[" in expr and "]" in expr:
        start = expr.find("[")
        end = expr.find("]")
        obj_name = expr[:start]
        key_expr = expr[start+1:end]

        obj = context.get(obj_name, [])
        if key_expr.isdigit():
            key = int(key_expr)
            if type(obj) in ["list", "tuple"] and key >= 0 and key < len(obj):
                return obj[key]
        else:
            key = key_expr.strip('"').strip("'")
            if type(obj) == "dict":
                return obj.get(key, "")
        return ""

    # Simple variable
    return context.get(expr, "")

def eval_condition(condition, context):
    """Evaluate a boolean condition"""
    # Simple truthy check
    value = eval_variable(condition, context)
    return is_truthy(value)

def apply_filter(filter_expr, value, context):
    """Apply a filter to a value"""
    # Parse filter name and args
    if "(" in filter_expr:
        paren = filter_expr.find("(")
        filter_name = filter_expr[:paren]
        args_str = filter_expr[paren+1:-1]
        args = [eval_simple_expr(args_str, context)]
    else:
        filter_name = filter_expr
        args = []

    # Apply filter
    if filter_name == "upper":
        return str(value).upper()
    elif filter_name == "lower":
        return str(value).lower()
    elif filter_name == "title":
        return str(value).title()
    elif filter_name == "capitalize":
        s = str(value)
        return s[0].upper() + s[1:].lower() if len(s) > 0 else s
    elif filter_name == "default":
        if not value or value == "":
            return args[0] if len(args) > 0 else ""
        return value
    elif filter_name == "length":
        if type(value) in ["list", "tuple", "dict", "string"]:
            return len(value)
        return 0
    elif filter_name == "join":
        sep = str(args[0]) if len(args) > 0 else ""
        if type(value) in ["list", "tuple"]:
            return sep.join([str(x) for x in value])
        return str(value)

    return value

def is_truthy(value):
    """Check if value is truthy in Jinja context"""
    if value == None or value == False or value == "" or value == 0:
        return False
    if type(value) in ["list", "tuple", "dict"] and len(value) == 0:
        return False
    return True

def html_escape(text):
    """Escape HTML special characters"""
    text = str(text)
    text = text.replace("&", "&amp;")
    text = text.replace("<", "&lt;")
    text = text.replace(">", "&gt;")
    text = text.replace('"', "&quot;")
    text = text.replace("'", "&#x27;")
    return text

# No need for exports - render function is directly available
