package template

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// TestJinjaIntegratedQuery demonstrates a real-world scenario:
// 1. Starlark endpoint queries database for products
// 2. Main template renders product list
// 3. Template includes header/footer partials from database
// 4. All templates support inheritance (base layout)
func TestJinjaIntegratedQuery(t *testing.T) {
	// Setup: Create in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create documents table for templates
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
		t.Fatalf("Failed to create documents table: %v", err)
	}

	// Create a products table (user data)
	_, err = db.Exec(`
		CREATE TABLE products (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			price REAL NOT NULL,
			category TEXT NOT NULL,
			in_stock INTEGER DEFAULT 1
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create products table: %v", err)
	}

	// Insert sample products
	_, err = db.Exec(`
		INSERT INTO products (name, price, category, in_stock) VALUES
			('MacBook Pro', 2499.99, 'Computers', 1),
			('iPhone 15', 999.99, 'Phones', 1),
			('AirPods Pro', 249.99, 'Audio', 1),
			('iPad Air', 599.99, 'Tablets', 0),
			('Apple Watch', 399.99, 'Wearables', 1)
	`)
	if err != nil {
		t.Fatalf("Failed to insert products: %v", err)
	}

	now := time.Now().Unix()

	// Template 1: Base layout (with blocks)
	baseTemplate := `<!DOCTYPE html>
<html>
<head>
	<title>{% block title %}My Store{% endblock %}</title>
	<style>{% block styles %}{% endblock %}</style>
</head>
<body>
	{% include "templates/partials/header.html" %}

	<main>
		{% block content %}
		<p>Default content</p>
		{% endblock %}
	</main>

	{% include "templates/partials/footer.html" %}
</body>
</html>`

	_, err = db.Exec(`
		INSERT INTO _wce_documents (id, content, content_type, created_at, modified_at, created_by, modified_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "templates/base.html", baseTemplate, "text/html+jinja", now, now, "test-user", "test-user")
	if err != nil {
		t.Fatalf("Failed to insert base template: %v", err)
	}

	// Template 2: Header partial
	headerTemplate := `<header>
	<h1>{{ store_name }}</h1>
	<nav>
		<a href="/">Home</a> |
		<a href="/products">Products</a> |
		<a href="/cart">Cart ({{ cart_count|default("0") }})</a>
	</nav>
</header>`

	_, err = db.Exec(`
		INSERT INTO _wce_documents (id, content, content_type, created_at, modified_at, created_by, modified_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "templates/partials/header.html", headerTemplate, "text/html+jinja", now, now, "test-user", "test-user")
	if err != nil {
		t.Fatalf("Failed to insert header template: %v", err)
	}

	// Template 3: Footer partial
	footerTemplate := `<footer>
	<p>&copy; {{ year }} {{ store_name }}. All rights reserved.</p>
	<p>Total products: {{ total_products }}</p>
</footer>`

	_, err = db.Exec(`
		INSERT INTO _wce_documents (id, content, content_type, created_at, modified_at, created_by, modified_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "templates/partials/footer.html", footerTemplate, "text/html+jinja", now, now, "test-user", "test-user")
	if err != nil {
		t.Fatalf("Failed to insert footer template: %v", err)
	}

	// Template 4: Product list page (extends base, includes partials)
	productListTemplate := `{% extends "templates/base.html" %}

{% block title %}{{ category|upper }} Products - {{ store_name }}{% endblock %}

{% block styles %}
body { font-family: Arial, sans-serif; }
.product { border: 1px solid #ccc; padding: 10px; margin: 10px 0; }
.out-of-stock { opacity: 0.5; }
{% endblock %}

{% block content %}
<h2>{{ category }} Products</h2>

{% if products %}
	<p>Showing {{ products|length }} products</p>

	{% for product in products %}
	<div class="product{% if not product.in_stock %} out-of-stock{% endif %}">
		<h3>{{ loop.index }}. {{ product.name }}</h3>
		<p>Price: ${{ product.price }}</p>
		<p>Category: {{ product.category }}</p>
		{% if product.in_stock %}
			<button>Add to Cart</button>
		{% else %}
			<span>Out of Stock</span>
		{% endif %}
	</div>
	{% endfor %}
{% else %}
	<p>No products found in this category.</p>
{% endif %}

{% endblock %}`

	_, err = db.Exec(`
		INSERT INTO _wce_documents (id, content, content_type, created_at, modified_at, created_by, modified_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "templates/pages/products.html", productListTemplate, "text/html+jinja", now, now, "test-user", "test-user")
	if err != nil {
		t.Fatalf("Failed to insert product list template: %v", err)
	}

	// SIMULATE STARLARK ENDPOINT QUERYING DATABASE
	// In real usage, this would be done by a Starlark script:
	// db.query("SELECT * FROM products WHERE in_stock = 1")

	rows, err := db.Query(`
		SELECT name, price, category, in_stock
		FROM products
		WHERE category = ? OR in_stock = 1
		ORDER BY price DESC
	`, "Phones")
	if err != nil {
		t.Fatalf("Failed to query products: %v", err)
	}
	defer rows.Close()

	// Build products array (what Starlark would return)
	var products []interface{}
	for rows.Next() {
		var name, category string
		var price float64
		var inStock int

		if err := rows.Scan(&name, &price, &category, &inStock); err != nil {
			t.Fatalf("Failed to scan row: %v", err)
		}

		products = append(products, map[string]interface{}{
			"name":     name,
			"price":    price,
			"category": category,
			"in_stock": inStock == 1,
		})
	}

	// Get total product count
	var totalProducts int
	db.QueryRow("SELECT COUNT(*) FROM products").Scan(&totalProducts)

	// NOW RENDER THE TEMPLATE with query results
	ctx := context.Background()

	// Create template loader that reads from database
	loader := DocumentLoader(db)

	// Prepare context with query results + global data
	variables := map[string]interface{}{
		"store_name":     "Apple Store",
		"category":       "All",
		"products":       products,
		"cart_count":     3,
		"year":           2025,
		"total_products": totalProducts,
	}

	renderCtx := &RenderContext{
		Variables: variables,
		Loader:    loader,
	}

	// Load and render the main template
	var templateSource string
	err = db.QueryRow("SELECT content FROM _wce_documents WHERE id = ?", "templates/pages/products.html").Scan(&templateSource)
	if err != nil {
		t.Fatalf("Failed to load template: %v", err)
	}

	result, err := RenderTemplate(ctx, templateSource, renderCtx)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	// VERIFY THE RESULT

	// Check base layout elements
	if !contains(result, "<!DOCTYPE html>") {
		t.Errorf("Missing DOCTYPE from base template")
	}
	if !contains(result, "<html>") {
		t.Errorf("Missing html tag from base template")
	}

	// Check header partial was included
	if !contains(result, "<header>") {
		t.Errorf("Missing header tag from header partial")
	}
	if !contains(result, "<h1>Apple Store</h1>") {
		t.Errorf("Missing store name from header partial")
	}
	if !contains(result, "Cart (3)") {
		t.Errorf("Missing cart count from header partial")
	}

	// Check footer partial was included
	if !contains(result, "<footer>") {
		t.Errorf("Missing footer tag from footer partial")
	}
	if !contains(result, "© 2025 Apple Store") {
		t.Errorf("Missing copyright from footer partial")
	}
	if !contains(result, "Total products: 5") {
		t.Errorf("Missing product count from footer partial")
	}

	// Check title block override
	if !contains(result, "<title>ALL Products - Apple Store</title>") {
		t.Errorf("Missing title block override. Got: %s", extractTitle(result))
	}

	// Check styles block
	if !contains(result, "font-family: Arial") {
		t.Errorf("Missing styles from styles block")
	}

	// Check product list rendering
	if !contains(result, "<h2>All Products</h2>") {
		t.Errorf("Missing category heading")
	}
	if !contains(result, "Showing 5 products") {
		t.Errorf("Missing product count. Expected 'Showing 5 products'")
	}

	// Check individual products (from database query)
	if !contains(result, "MacBook Pro") {
		t.Errorf("Missing MacBook Pro from product list")
	}
	if !contains(result, "$2499.99") {
		t.Errorf("Missing MacBook Pro price")
	}
	if !contains(result, "iPhone 15") {
		t.Errorf("Missing iPhone 15 from product list")
	}

	// Check loop index
	if !contains(result, "1. MacBook Pro") {
		t.Errorf("Missing loop index for first product")
	}

	// Check conditional rendering (in stock vs out of stock)
	if !contains(result, "Add to Cart") {
		t.Errorf("Missing 'Add to Cart' button for in-stock items")
	}
	if !contains(result, "Out of Stock") {
		t.Errorf("Missing 'Out of Stock' text for unavailable items")
	}
	if !contains(result, "class=\"product out-of-stock\"") {
		t.Errorf("Missing CSS class for out-of-stock product")
	}

	t.Logf("✓ Template inheritance working (extends)")
	t.Logf("✓ Partial templates working (include)")
	t.Logf("✓ Database query results rendering")
	t.Logf("✓ Filters working (upper, length, default)")
	t.Logf("✓ Loops with conditionals working")
	t.Logf("✓ All templates loaded from SQLite")
	t.Logf("\n--- RENDERED OUTPUT (first 500 chars) ---\n%s\n", truncate(result, 500))
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) >= len(substr) &&
			(s[:len(substr)] == substr ||
			 s[len(s)-len(substr):] == substr ||
			 findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func extractTitle(html string) string {
	start := 0
	for i := 0; i < len(html)-7; i++ {
		if html[i:i+7] == "<title>" {
			start = i + 7
			break
		}
	}
	if start == 0 {
		return ""
	}

	for i := start; i < len(html)-8; i++ {
		if html[i:i+8] == "</title>" {
			return html[start:i]
		}
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
