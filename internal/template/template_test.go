package template

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Test basic variable rendering
func TestRenderSimpleVariable(t *testing.T) {
	ctx := context.Background()
	template := "Hello {{ name }}!"
	variables := map[string]interface{}{
		"name": "World",
	}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "Hello World!"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test filter: upper
func TestRenderFilterUpper(t *testing.T) {
	ctx := context.Background()
	template := "{{ name|upper }}"
	variables := map[string]interface{}{
		"name": "alice",
	}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "ALICE"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test filter: lower
func TestRenderFilterLower(t *testing.T) {
	ctx := context.Background()
	template := "{{ name|lower }}"
	variables := map[string]interface{}{
		"name": "BOB",
	}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "bob"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test filter: default
func TestRenderFilterDefault(t *testing.T) {
	ctx := context.Background()
	template := "{{ missing|default('Guest') }}"
	variables := map[string]interface{}{}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "Guest"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test for loop
func TestRenderForLoop(t *testing.T) {
	ctx := context.Background()
	template := "{% for x in items %}{{ x }}{% endfor %}"
	variables := map[string]interface{}{
		"items": []interface{}{1, 2, 3},
	}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "123"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test for loop with loop.index
func TestRenderForLoopIndex(t *testing.T) {
	ctx := context.Background()
	template := "{% for x in items %}{{ loop.index }}{% endfor %}"
	variables := map[string]interface{}{
		"items": []interface{}{"a", "b", "c"},
	}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "123"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test if statement (true)
func TestRenderIfTrue(t *testing.T) {
	ctx := context.Background()
	template := "{% if show %}yes{% else %}no{% endif %}"
	variables := map[string]interface{}{
		"show": true,
	}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "yes"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test if statement (false)
func TestRenderIfFalse(t *testing.T) {
	ctx := context.Background()
	template := "{% if show %}yes{% else %}no{% endif %}"
	variables := map[string]interface{}{
		"show": false,
	}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "no"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test HTML escaping
func TestRenderHTMLEscape(t *testing.T) {
	ctx := context.Background()
	template := "{{ html }}"
	variables := map[string]interface{}{
		"html": "<script>alert('xss')</script>",
	}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	// Should be escaped
	if !strings.Contains(result, "&lt;script&gt;") {
		t.Errorf("Expected HTML to be escaped, got: %s", result)
	}
	if strings.Contains(result, "<script>") {
		t.Errorf("HTML should be escaped, got: %s", result)
	}
}

// Test comments (should be removed)
func TestRenderComments(t *testing.T) {
	ctx := context.Background()
	template := "Hello {# this is a comment #}World"
	variables := map[string]interface{}{}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "Hello World"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test set statement
func TestRenderSet(t *testing.T) {
	ctx := context.Background()
	template := "{% set greeting = 'Hello' %}{{ greeting }} World"
	variables := map[string]interface{}{}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "Hello World"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test template inheritance (extends)
func TestRenderExtends(t *testing.T) {
	ctx := context.Background()

	// Base template
	baseTemplate := `<!DOCTYPE html>
<html>
<head><title>{% block title %}Default{% endblock %}</title></head>
<body>{% block content %}{% endblock %}</body>
</html>`

	// Child template
	childTemplate := `{% extends "base.html" %}
{% block title %}My Page{% endblock %}
{% block content %}<h1>Welcome</h1>{% endblock %}`

	// Template loader
	loader := func(name string) (string, error) {
		if name == "base.html" {
			return baseTemplate, nil
		}
		return "", fmt.Errorf("template not found: %s", name)
	}

	variables := map[string]interface{}{}

	renderCtx := &RenderContext{
		Variables: variables,
		Loader:    loader,
	}

	result, err := RenderTemplate(ctx, childTemplate, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	// Should contain child content
	if !strings.Contains(result, "My Page") {
		t.Errorf("Expected 'My Page' in result, got: %s", result)
	}
	if !strings.Contains(result, "<h1>Welcome</h1>") {
		t.Errorf("Expected '<h1>Welcome</h1>' in result, got: %s", result)
	}
	if !strings.Contains(result, "<!DOCTYPE html>") {
		t.Errorf("Expected '<!DOCTYPE html>' in result, got: %s", result)
	}
}

// Test include statement
func TestRenderInclude(t *testing.T) {
	ctx := context.Background()

	mainTemplate := `<div>{% include "header.html" %}<p>Content</p></div>`
	headerTemplate := `<header>Header</header>`

	loader := func(name string) (string, error) {
		if name == "header.html" {
			return headerTemplate, nil
		}
		return "", fmt.Errorf("template not found: %s", name)
	}

	variables := map[string]interface{}{}

	renderCtx := &RenderContext{
		Variables: variables,
		Loader:    loader,
	}

	result, err := RenderTemplate(ctx, mainTemplate, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "<div><header>Header</header><p>Content</p></div>"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Test complex template with multiple features
func TestRenderComplex(t *testing.T) {
	ctx := context.Background()

	template := `<html>
<head><title>{{ title|upper }}</title></head>
<body>
<h1>{{ title }}</h1>
{% if user %}
  <p>Welcome, {{ user.name }}!</p>
{% else %}
  <p>Please log in.</p>
{% endif %}
<ul>
{% for item in items %}
  <li>{{ loop.index }}. {{ item.name }} - {{ item.price }}</li>
{% endfor %}
</ul>
</body>
</html>`

	variables := map[string]interface{}{
		"title": "Shop",
		"user": map[string]interface{}{
			"name": "Alice",
		},
		"items": []interface{}{
			map[string]interface{}{"name": "Apple", "price": 1.5},
			map[string]interface{}{"name": "Banana", "price": 0.75},
		},
	}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	result, err := RenderTemplate(ctx, template, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	// Check key elements are present
	if !strings.Contains(result, "<title>SHOP</title>") {
		t.Errorf("Expected '<title>SHOP</title>' in result")
	}
	if !strings.Contains(result, "Welcome, Alice!") {
		t.Errorf("Expected 'Welcome, Alice!' in result")
	}
	if !strings.Contains(result, "1. Apple - 1.5") {
		t.Errorf("Expected '1. Apple - 1.5' in result")
	}
	if !strings.Contains(result, "2. Banana - 0.75") {
		t.Errorf("Expected '2. Banana - 0.75' in result")
	}
}

// Test RenderTemplateFromDB with in-memory database
func TestRenderTemplateFromDB(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create documents table
	_, err = db.Exec(`
		CREATE TABLE _wce_documents (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			content_type TEXT NOT NULL,
			is_binary INTEGER DEFAULT 0,
			searchable INTEGER DEFAULT 1,
			created_at INTEGER NOT NULL,
			modified_at INTEGER NOT NULL,
			created_by TEXT NOT NULL,
			modified_by TEXT NOT NULL,
			version INTEGER DEFAULT 1
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert a base template
	now := time.Now().Unix()
	_, err = db.Exec(`
		INSERT INTO _wce_documents (id, content, content_type, created_at, modified_at, created_by, modified_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "templates/base.html", "<html><body>{% block content %}Default{% endblock %}</body></html>",
		"text/html+jinja", now, now, "test-user", "test-user")
	if err != nil {
		t.Fatalf("Failed to insert base template: %v", err)
	}

	// Insert a page template that extends base
	_, err = db.Exec(`
		INSERT INTO _wce_documents (id, content, content_type, created_at, modified_at, created_by, modified_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "templates/pages/home.html", "{% extends 'templates/base.html' %}{% block content %}<h1>{{ title }}</h1>{% endblock %}",
		"text/html+jinja", now, now, "test-user", "test-user")
	if err != nil {
		t.Fatalf("Failed to insert page template: %v", err)
	}

	// Render the page template
	ctx := context.Background()
	variables := map[string]interface{}{
		"title": "Home Page",
	}

	result, err := RenderTemplateFromDB(ctx, db, "templates/pages/home.html", variables)
	if err != nil {
		t.Fatalf("RenderTemplateFromDB failed: %v", err)
	}

	// Verify result contains expected content
	if !strings.Contains(result, "<html>") {
		t.Errorf("Expected '<html>' in result")
	}
	if !strings.Contains(result, "<h1>Home Page</h1>") {
		t.Errorf("Expected '<h1>Home Page</h1>' in result")
	}
	if strings.Contains(result, "Default") {
		t.Errorf("Should not contain 'Default' (should be overridden)")
	}
}

// Test timeout handling
func TestRenderTimeout(t *testing.T) {
	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	template := "Hello {{ name }}"
	variables := map[string]interface{}{
		"name": "World",
	}

	renderCtx := &RenderContext{
		Variables: variables,
	}

	_, err := RenderTemplate(ctx, template, renderCtx)
	if err == nil {
		t.Errorf("Expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("Expected 'expired' error, got: %v", err)
	}
}

// Test error handling: template not found
func TestRenderTemplateNotFound(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create documents table
	_, err = db.Exec(`
		CREATE TABLE _wce_documents (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			content_type TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			modified_at INTEGER NOT NULL,
			created_by TEXT NOT NULL,
			modified_by TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	ctx := context.Background()
	variables := map[string]interface{}{}

	_, err = RenderTemplateFromDB(ctx, db, "nonexistent.html", variables)
	if err == nil {
		t.Errorf("Expected error for missing template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}
