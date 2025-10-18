# jinja.star - Jinja2-compatible template engine in pure Starlark
#
# This module implements a Jinja2-style template parser and renderer
# entirely in Starlark, with no external dependencies.
#
# Supported features:
# - Variable output: {{ variable }}
# - Filters: {{ var|upper }}, {{ var|default("value") }}
# - Control structures: {% if %}, {% for %}
# - Template inheritance: {% extends %}, {% block %}
# - Includes: {% include %}
# - Comments: {# comment #}
# - Auto-escaping of HTML

# Token types
TOKEN_TEXT = "TEXT"
TOKEN_VAR_BEGIN = "VAR_BEGIN"
TOKEN_VAR_END = "VAR_END"
TOKEN_STMT_BEGIN = "STMT_BEGIN"
TOKEN_STMT_END = "STMT_END"
TOKEN_COMMENT_BEGIN = "COMMENT_BEGIN"
TOKEN_COMMENT_END = "COMMENT_END"
TOKEN_IDENTIFIER = "IDENTIFIER"
TOKEN_STRING = "STRING"
TOKEN_NUMBER = "NUMBER"
TOKEN_FILTER = "FILTER"
TOKEN_DOT = "DOT"
TOKEN_LBRACKET = "LBRACKET"
TOKEN_RBRACKET = "RBRACKET"
TOKEN_LPAREN = "LPAREN"
TOKEN_RPAREN = "RPAREN"
TOKEN_COMMA = "COMMA"
TOKEN_PIPE = "PIPE"
TOKEN_ASSIGN = "ASSIGN"

def tokenize(template):
    """
    Lexer: Convert template string into tokens.
    Returns list of token dicts: {"type": ..., "value": ..., "pos": ...}
    """
    tokens = []
    pos = 0
    length = len(template)

    while pos < length:
        # Check for {# comment #}
        if template[pos:pos+2] == "{#":
            end = template.find("#}", pos + 2)
            if end == -1:
                fail("Unclosed comment at position %d" % pos)
            # Skip comments entirely
            pos = end + 2
            continue

        # Check for {{ variable }}
        if template[pos:pos+2] == "{{":
            tokens.append({"type": TOKEN_VAR_BEGIN, "value": "{{", "pos": pos})
            pos += 2
            # Tokenize inside {{ }}
            expr_start = pos
            depth = 1
            while pos < length and depth > 0:
                if template[pos:pos+2] == "}}":
                    depth -= 1
                    if depth == 0:
                        break
                pos += 1

            # Parse expression tokens
            expr = template[expr_start:pos].strip()
            tokens.extend(tokenize_expr(expr, expr_start))
            tokens.append({"type": TOKEN_VAR_END, "value": "}}", "pos": pos})
            pos += 2
            continue

        # Check for {% statement %}
        if template[pos:pos+2] == "{%":
            tokens.append({"type": TOKEN_STMT_BEGIN, "value": "{%", "pos": pos})
            pos += 2
            # Tokenize inside {% %}
            stmt_start = pos
            depth = 1
            while pos < length and depth > 0:
                if template[pos:pos+2] == "%}":
                    depth -= 1
                    if depth == 0:
                        break
                pos += 1

            # Parse statement tokens
            stmt = template[stmt_start:pos].strip()
            tokens.extend(tokenize_expr(stmt, stmt_start))
            tokens.append({"type": TOKEN_STMT_END, "value": "%}", "pos": pos})
            pos += 2
            continue

        # Plain text
        text_start = pos
        while pos < length:
            if template[pos:pos+2] in ["{{", "{%", "{#"]:
                break
            pos += 1

        if pos > text_start:
            tokens.append({"type": TOKEN_TEXT, "value": template[text_start:pos], "pos": text_start})

    return tokens

def tokenize_expr(expr, offset):
    """Tokenize an expression (inside {{ }} or {% %})"""
    tokens = []
    pos = 0
    length = len(expr)

    while pos < length:
        # Skip whitespace
        while pos < length and expr[pos] in " \t\n\r":
            pos += 1

        if pos >= length:
            break

        ch = expr[pos]

        # Operators and delimiters
        if ch == "|":
            tokens.append({"type": TOKEN_PIPE, "value": "|", "pos": offset + pos})
            pos += 1
        elif ch == ".":
            tokens.append({"type": TOKEN_DOT, "value": ".", "pos": offset + pos})
            pos += 1
        elif ch == "[":
            tokens.append({"type": TOKEN_LBRACKET, "value": "[", "pos": offset + pos})
            pos += 1
        elif ch == "]":
            tokens.append({"type": TOKEN_RBRACKET, "value": "]", "pos": offset + pos})
            pos += 1
        elif ch == "(":
            tokens.append({"type": TOKEN_LPAREN, "value": "(", "pos": offset + pos})
            pos += 1
        elif ch == ")":
            tokens.append({"type": TOKEN_RPAREN, "value": ")", "pos": offset + pos})
            pos += 1
        elif ch == ",":
            tokens.append({"type": TOKEN_COMMA, "value": ",", "pos": offset + pos})
            pos += 1
        elif ch == "=":
            tokens.append({"type": TOKEN_ASSIGN, "value": "=", "pos": offset + pos})
            pos += 1

        # String literals
        elif ch in ['"', "'"]:
            quote = ch
            start = pos
            pos += 1
            while pos < length and expr[pos] != quote:
                if expr[pos] == "\\":
                    pos += 2
                else:
                    pos += 1
            if pos >= length:
                fail("Unclosed string at position %d" % (offset + start))
            pos += 1  # Skip closing quote
            tokens.append({"type": TOKEN_STRING, "value": expr[start+1:pos-1], "pos": offset + start})

        # Numbers
        elif ch.isdigit():
            start = pos
            while pos < length and (expr[pos].isdigit() or expr[pos] == "."):
                pos += 1
            num_str = expr[start:pos]
            if "." in num_str:
                tokens.append({"type": TOKEN_NUMBER, "value": float(num_str), "pos": offset + start})
            else:
                tokens.append({"type": TOKEN_NUMBER, "value": int(num_str), "pos": offset + start})

        # Identifiers and keywords
        elif ch.isalpha() or ch == "_":
            start = pos
            while pos < length and (expr[pos].isalnum() or expr[pos] == "_"):
                pos += 1
            ident = expr[start:pos]
            tokens.append({"type": TOKEN_IDENTIFIER, "value": ident, "pos": offset + start})

        else:
            fail("Unexpected character '%s' at position %d" % (ch, offset + pos))

    return tokens

def parse(tokens):
    """
    Parser: Build Abstract Syntax Tree from tokens.
    Returns list of AST nodes.
    """
    ast = []
    i = 0

    while i < len(tokens):
        token = tokens[i]

        if token["type"] == TOKEN_TEXT:
            ast.append({"type": "text", "value": token["value"]})
            i += 1

        elif token["type"] == TOKEN_VAR_BEGIN:
            # Parse variable expression
            i += 1
            expr_tokens = []
            while i < len(tokens) and tokens[i]["type"] != TOKEN_VAR_END:
                expr_tokens.append(tokens[i])
                i += 1

            if i >= len(tokens):
                fail("Unclosed variable tag")

            # Parse the expression
            var_node, _ = parse_expr(expr_tokens, 0)
            ast.append({"type": "var", "expr": var_node})
            i += 1  # Skip VAR_END

        elif token["type"] == TOKEN_STMT_BEGIN:
            # Parse statement
            i += 1
            stmt_tokens = []
            while i < len(tokens) and tokens[i]["type"] != TOKEN_STMT_END:
                stmt_tokens.append(tokens[i])
                i += 1

            if i >= len(tokens):
                fail("Unclosed statement tag")

            # Parse the statement
            stmt_node, new_i = parse_statement(tokens, i + 1, stmt_tokens)
            ast.append(stmt_node)
            i = new_i

        else:
            i += 1

    return ast

def parse_statement(all_tokens, pos, stmt_tokens):
    """Parse a statement like if, for, set, extends, etc."""
    if len(stmt_tokens) == 0:
        fail("Empty statement")

    keyword = stmt_tokens[0]
    if keyword["type"] != TOKEN_IDENTIFIER:
        fail("Expected keyword, got %s" % keyword["type"])

    kw = keyword["value"]

    # {% if condition %}
    if kw == "if":
        return parse_if_statement(all_tokens, pos, stmt_tokens[1:])

    # {% for var in iterable %}
    elif kw == "for":
        return parse_for_statement(all_tokens, pos, stmt_tokens[1:])

    # {% set var = value %}
    elif kw == "set":
        return parse_set_statement(stmt_tokens[1:]), pos

    # {% extends "template" %}
    elif kw == "extends":
        return parse_extends_statement(stmt_tokens[1:]), pos

    # {% block name %}
    elif kw == "block":
        return parse_block_statement(all_tokens, pos, stmt_tokens[1:])

    # {% include "template" %}
    elif kw == "include":
        return parse_include_statement(stmt_tokens[1:]), pos

    # {% endif %}, {% endfor %}, {% endblock %}, {% elif %}, {% else %}
    elif kw in ["endif", "endfor", "endblock", "elif", "else"]:
        # These are handled by their parent statements
        return None, pos

    else:
        fail("Unknown statement: %s" % kw)

def parse_if_statement(all_tokens, pos, condition_tokens):
    """Parse if/elif/else statement"""
    condition_expr, _ = parse_expr(condition_tokens, 0)

    # Parse body until {% elif %}, {% else %}, or {% endif %}
    body = []
    elif_clauses = []
    else_body = None

    while pos < len(all_tokens):
        token = all_tokens[pos]

        if token["type"] == TOKEN_STMT_BEGIN:
            # Check if it's elif, else, or endif
            next_pos = pos + 1
            if next_pos < len(all_tokens) and all_tokens[next_pos]["type"] == TOKEN_IDENTIFIER:
                kw = all_tokens[next_pos]["value"]

                if kw == "endif":
                    # Skip to after %}
                    while pos < len(all_tokens) and all_tokens[pos]["type"] != TOKEN_STMT_END:
                        pos += 1
                    pos += 1
                    break

                elif kw == "elif":
                    # Parse elif condition
                    elif_tokens = []
                    pos = next_pos + 1
                    while pos < len(all_tokens) and all_tokens[pos]["type"] != TOKEN_STMT_END:
                        elif_tokens.append(all_tokens[pos])
                        pos += 1
                    pos += 1  # Skip %}

                    # Parse elif body
                    elif_body, new_pos = parse_block_until(all_tokens, pos, ["elif", "else", "endif"])
                    elif_cond, _ = parse_expr(elif_tokens, 0)
                    elif_clauses.append({"condition": elif_cond, "body": elif_body})
                    pos = new_pos
                    continue

                elif kw == "else":
                    # Skip to after %}
                    while pos < len(all_tokens) and all_tokens[pos]["type"] != TOKEN_STMT_END:
                        pos += 1
                    pos += 1

                    # Parse else body
                    else_body, new_pos = parse_block_until(all_tokens, pos, ["endif"])
                    pos = new_pos
                    continue

        # Add to body
        if token["type"] == TOKEN_TEXT:
            body.append({"type": "text", "value": token["value"]})
            pos += 1
        elif token["type"] == TOKEN_VAR_BEGIN:
            # Parse variable
            pos += 1
            expr_tokens = []
            while pos < len(all_tokens) and all_tokens[pos]["type"] != TOKEN_VAR_END:
                expr_tokens.append(all_tokens[pos])
                pos += 1
            var_node, _ = parse_expr(expr_tokens, 0)
            body.append({"type": "var", "expr": var_node})
            pos += 1
        elif token["type"] == TOKEN_STMT_BEGIN:
            # Nested statement
            pos += 1
            stmt_tokens = []
            while pos < len(all_tokens) and all_tokens[pos]["type"] != TOKEN_STMT_END:
                stmt_tokens.append(all_tokens[pos])
                pos += 1
            pos += 1
            stmt_node, new_pos = parse_statement(all_tokens, pos, stmt_tokens)
            if stmt_node:
                body.append(stmt_node)
            pos = new_pos
        else:
            pos += 1

    node = {
        "type": "if",
        "condition": condition_expr,
        "body": body,
    }
    if len(elif_clauses) > 0:
        node["elif"] = elif_clauses
    if else_body != None:
        node["else"] = else_body

    return node, pos

def parse_for_statement(all_tokens, pos, for_tokens):
    """Parse for loop: for var in iterable"""
    # Expect: var in iterable
    if len(for_tokens) < 3:
        fail("Invalid for statement")

    var_name = for_tokens[0]["value"]
    if for_tokens[1]["value"] != "in":
        fail("Expected 'in' in for statement")

    iterable_expr, _ = parse_expr(for_tokens[2:], 0)

    # Parse body until {% endfor %}
    body = []

    while pos < len(all_tokens):
        token = all_tokens[pos]

        if token["type"] == TOKEN_STMT_BEGIN:
            next_pos = pos + 1
            if next_pos < len(all_tokens) and all_tokens[next_pos]["type"] == TOKEN_IDENTIFIER:
                if all_tokens[next_pos]["value"] == "endfor":
                    # Skip to after %}
                    while pos < len(all_tokens) and all_tokens[pos]["type"] != TOKEN_STMT_END:
                        pos += 1
                    pos += 1
                    break

        # Add to body
        if token["type"] == TOKEN_TEXT:
            body.append({"type": "text", "value": token["value"]})
            pos += 1
        elif token["type"] == TOKEN_VAR_BEGIN:
            pos += 1
            expr_tokens = []
            while pos < len(all_tokens) and all_tokens[pos]["type"] != TOKEN_VAR_END:
                expr_tokens.append(all_tokens[pos])
                pos += 1
            var_node, _ = parse_expr(expr_tokens, 0)
            body.append({"type": "var", "expr": var_node})
            pos += 1
        elif token["type"] == TOKEN_STMT_BEGIN:
            # Nested statement
            pos += 1
            stmt_tokens = []
            while pos < len(all_tokens) and all_tokens[pos]["type"] != TOKEN_STMT_END:
                stmt_tokens.append(all_tokens[pos])
                pos += 1
            pos += 1
            stmt_node, new_pos = parse_statement(all_tokens, pos, stmt_tokens)
            if stmt_node:
                body.append(stmt_node)
            pos = new_pos
        else:
            pos += 1

    return {
        "type": "for",
        "var": var_name,
        "iterable": iterable_expr,
        "body": body,
    }, pos

def parse_block_until(all_tokens, pos, end_keywords):
    """Parse tokens until one of the end keywords is found"""
    body = []

    while pos < len(all_tokens):
        token = all_tokens[pos]

        if token["type"] == TOKEN_STMT_BEGIN:
            next_pos = pos + 1
            if next_pos < len(all_tokens) and all_tokens[next_pos]["type"] == TOKEN_IDENTIFIER:
                if all_tokens[next_pos]["value"] in end_keywords:
                    return body, pos

        if token["type"] == TOKEN_TEXT:
            body.append({"type": "text", "value": token["value"]})
            pos += 1
        elif token["type"] == TOKEN_VAR_BEGIN:
            pos += 1
            expr_tokens = []
            while pos < len(all_tokens) and all_tokens[pos]["type"] != TOKEN_VAR_END:
                expr_tokens.append(all_tokens[pos])
                pos += 1
            var_node, _ = parse_expr(expr_tokens, 0)
            body.append({"type": "var", "expr": var_node})
            pos += 1
        elif token["type"] == TOKEN_STMT_BEGIN:
            pos += 1
            stmt_tokens = []
            while pos < len(all_tokens) and all_tokens[pos]["type"] != TOKEN_STMT_END:
                stmt_tokens.append(all_tokens[pos])
                pos += 1
            pos += 1
            stmt_node, new_pos = parse_statement(all_tokens, pos, stmt_tokens)
            if stmt_node:
                body.append(stmt_node)
            pos = new_pos
        else:
            pos += 1

    return body, pos

def parse_set_statement(tokens):
    """Parse {% set var = value %}"""
    if len(tokens) < 3:
        fail("Invalid set statement")

    var_name = tokens[0]["value"]
    if tokens[1]["type"] != TOKEN_ASSIGN:
        fail("Expected '=' in set statement")

    value_expr, _ = parse_expr(tokens[2:], 0)

    return {
        "type": "set",
        "var": var_name,
        "value": value_expr,
    }

def parse_extends_statement(tokens):
    """Parse {% extends "template" %}"""
    if len(tokens) != 1 or tokens[0]["type"] != TOKEN_STRING:
        fail("extends requires a single string argument")

    return {
        "type": "extends",
        "parent": tokens[0]["value"],
    }

def parse_block_statement(all_tokens, pos, name_tokens):
    """Parse {% block name %} ... {% endblock %}"""
    if len(name_tokens) != 1 or name_tokens[0]["type"] != TOKEN_IDENTIFIER:
        fail("block requires a single name")

    block_name = name_tokens[0]["value"]

    # Parse body until {% endblock %}
    body, new_pos = parse_block_until(all_tokens, pos, ["endblock"])

    # Skip endblock
    while new_pos < len(all_tokens) and all_tokens[new_pos]["type"] != TOKEN_STMT_END:
        new_pos += 1
    new_pos += 1

    return {
        "type": "block",
        "name": block_name,
        "body": body,
    }, new_pos

def parse_include_statement(tokens):
    """Parse {% include "template" %}"""
    if len(tokens) != 1 or tokens[0]["type"] != TOKEN_STRING:
        fail("include requires a single string argument")

    return {
        "type": "include",
        "template": tokens[0]["value"],
    }

def parse_expr(tokens, pos):
    """Parse an expression (variable access, filters, etc.)"""
    if pos >= len(tokens):
        fail("Empty expression")

    # Start with primary expression (identifier, literal, etc.)
    expr, pos = parse_primary(tokens, pos)

    # Check for filters (|filter)
    while pos < len(tokens) and tokens[pos]["type"] == TOKEN_PIPE:
        pos += 1  # Skip pipe
        if pos >= len(tokens) or tokens[pos]["type"] != TOKEN_IDENTIFIER:
            fail("Expected filter name after |")

        filter_name = tokens[pos]["value"]
        pos += 1

        # Check for filter arguments
        filter_args = []
        if pos < len(tokens) and tokens[pos]["type"] == TOKEN_LPAREN:
            pos += 1  # Skip (
            while pos < len(tokens) and tokens[pos]["type"] != TOKEN_RPAREN:
                if tokens[pos]["type"] == TOKEN_COMMA:
                    pos += 1
                    continue
                arg_expr, pos = parse_primary(tokens, pos)
                filter_args.append(arg_expr)

            if pos >= len(tokens):
                fail("Unclosed filter arguments")
            pos += 1  # Skip )

        expr = {
            "type": "filter",
            "expr": expr,
            "filter": filter_name,
            "args": filter_args,
        }

    return expr, pos

def parse_primary(tokens, pos):
    """Parse a primary expression (identifier, literal, attribute access, subscript)"""
    if pos >= len(tokens):
        fail("Unexpected end of expression")

    token = tokens[pos]

    # Identifier
    if token["type"] == TOKEN_IDENTIFIER:
        expr = {"type": "identifier", "name": token["value"]}
        pos += 1

        # Check for attribute access (.attr) or subscript ([key])
        while pos < len(tokens):
            if tokens[pos]["type"] == TOKEN_DOT:
                pos += 1
                if pos >= len(tokens) or tokens[pos]["type"] != TOKEN_IDENTIFIER:
                    fail("Expected attribute name after .")
                expr = {
                    "type": "getattr",
                    "obj": expr,
                    "attr": tokens[pos]["value"],
                }
                pos += 1
            elif tokens[pos]["type"] == TOKEN_LBRACKET:
                pos += 1
                key_expr, pos = parse_expr(tokens[pos:], 0)
                pos = len(tokens) - len(tokens[pos:]) + pos  # Adjust position
                if pos >= len(tokens) or tokens[pos]["type"] != TOKEN_RBRACKET:
                    fail("Expected ] after subscript")
                expr = {
                    "type": "getitem",
                    "obj": expr,
                    "key": key_expr,
                }
                pos += 1
            else:
                break

        return expr, pos

    # String literal
    elif token["type"] == TOKEN_STRING:
        return {"type": "literal", "value": token["value"]}, pos + 1

    # Number literal
    elif token["type"] == TOKEN_NUMBER:
        return {"type": "literal", "value": token["value"]}, pos + 1

    else:
        fail("Unexpected token in expression: %s" % token["type"])

def render(template_source, context, template_loader=None):
    """
    Main entry point: Render a Jinja template with context data.

    Args:
        template_source: Template string
        context: Dict of variables
        template_loader: Function(name) -> template_source for extends/include

    Returns: Rendered HTML string
    """
    # Tokenize and parse
    tokens = tokenize(template_source)
    ast = parse(tokens)

    # Handle template inheritance (extends)
    if len(ast) > 0 and ast[0].get("type") == "extends":
        if not template_loader:
            fail("Template loader required for extends")

        parent_name = ast[0]["parent"]
        parent_source = template_loader(parent_name)

        # Extract blocks from child
        child_blocks = {}
        for node in ast:
            if node.get("type") == "block":
                child_blocks[node["name"]] = node["body"]

        # Parse parent and merge blocks
        parent_tokens = tokenize(parent_source)
        parent_ast = parse(parent_tokens)

        # Replace blocks in parent with child blocks
        merged_ast = merge_blocks(parent_ast, child_blocks)
        ast = merged_ast

    # Render the AST
    output = []
    for node in ast:
        output.append(render_node(node, context, template_loader))

    return "".join(output)

def merge_blocks(parent_ast, child_blocks):
    """Merge child blocks into parent AST"""
    result = []
    for node in parent_ast:
        if node.get("type") == "block":
            block_name = node["name"]
            if block_name in child_blocks:
                # Use child block content
                result.append({
                    "type": "block",
                    "name": block_name,
                    "body": child_blocks[block_name],
                })
            else:
                # Use parent block content
                result.append(node)
        else:
            result.append(node)
    return result

def render_node(node, context, template_loader):
    """Render a single AST node"""
    node_type = node.get("type")

    if node_type == "text":
        return node["value"]

    elif node_type == "var":
        value = eval_expr(node["expr"], context)
        # Auto-escape HTML
        return html_escape(to_string(value))

    elif node_type == "if":
        if is_truthy(eval_expr(node["condition"], context)):
            return render_body(node["body"], context, template_loader)

        # Check elif clauses
        for elif_clause in node.get("elif", []):
            if is_truthy(eval_expr(elif_clause["condition"], context)):
                return render_body(elif_clause["body"], context, template_loader)

        # Else clause
        if "else" in node:
            return render_body(node["else"], context, template_loader)

        return ""

    elif node_type == "for":
        items = eval_expr(node["iterable"], context)
        if type(items) not in ["list", "tuple"]:
            return ""

        output = []
        for i, item in enumerate(items):
            loop_context = dict(context)
            loop_context[node["var"]] = item
            loop_context["loop"] = {
                "index": i + 1,
                "index0": i,
                "first": i == 0,
                "last": i == len(items) - 1,
                "length": len(items),
            }
            output.append(render_body(node["body"], loop_context, template_loader))

        return "".join(output)

    elif node_type == "set":
        value = eval_expr(node["value"], context)
        context[node["var"]] = value
        return ""

    elif node_type == "block":
        return render_body(node["body"], context, template_loader)

    elif node_type == "include":
        if not template_loader:
            fail("Template loader required for include")

        included_source = template_loader(node["template"])
        return render(included_source, context, template_loader)

    elif node_type == "extends":
        # Handled in main render function
        return ""

    return ""

def render_body(body, context, template_loader):
    """Render a list of nodes"""
    output = []
    for node in body:
        output.append(render_node(node, context, template_loader))
    return "".join(output)

def eval_expr(expr, context):
    """Evaluate an expression"""
    expr_type = expr.get("type")

    if expr_type == "identifier":
        return context.get(expr["name"], "")

    elif expr_type == "literal":
        return expr["value"]

    elif expr_type == "getattr":
        obj = eval_expr(expr["obj"], context)
        attr = expr["attr"]
        if type(obj) == "dict":
            return obj.get(attr, "")
        # Starlark doesn't have getattr, so just return empty
        return ""

    elif expr_type == "getitem":
        obj = eval_expr(expr["obj"], context)
        key = eval_expr(expr["key"], context)
        if type(obj) == "dict":
            return obj.get(key, "")
        elif type(obj) in ["list", "tuple"]:
            if type(key) == "int" and key >= 0 and key < len(obj):
                return obj[key]
        return ""

    elif expr_type == "filter":
        value = eval_expr(expr["expr"], context)
        filter_name = expr["filter"]
        filter_args = [eval_expr(arg, context) for arg in expr.get("args", [])]
        return apply_filter(filter_name, value, filter_args)

    return ""

def apply_filter(name, value, args):
    """Apply a built-in filter"""
    if name == "upper":
        return to_string(value).upper()

    elif name == "lower":
        return to_string(value).lower()

    elif name == "default":
        if not value or value == "":
            return args[0] if len(args) > 0 else ""
        return value

    elif name == "length":
        if type(value) in ["list", "tuple", "dict", "string"]:
            return len(value)
        return 0

    elif name == "join":
        sep = to_string(args[0]) if len(args) > 0 else ""
        if type(value) in ["list", "tuple"]:
            return sep.join([to_string(x) for x in value])
        return to_string(value)

    elif name == "title":
        return to_string(value).title()

    elif name == "capitalize":
        s = to_string(value)
        if len(s) > 0:
            return s[0].upper() + s[1:].lower()
        return s

    # Unknown filter - return value unchanged
    return value

def is_truthy(value):
    """Check if value is truthy in Jinja context"""
    if value == None or value == False or value == "" or value == 0:
        return False
    if type(value) in ["list", "tuple", "dict"] and len(value) == 0:
        return False
    return True

def to_string(value):
    """Convert value to string"""
    if type(value) == "string":
        return value
    elif value == None:
        return ""
    else:
        return str(value)

def html_escape(text):
    """Escape HTML special characters"""
    text = to_string(text)
    text = text.replace("&", "&amp;")
    text = text.replace("<", "&lt;")
    text = text.replace(">", "&gt;")
    text = text.replace('"', "&quot;")
    text = text.replace("'", "&#x27;")
    return text

# Export public functions
jinja = struct(
    render = render,
    tokenize = tokenize,
    parse = parse,
    html_escape = html_escape,
)
