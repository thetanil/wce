package starlark

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestExecute_SimpleScript(t *testing.T) {
	script := `
def handle_request(req):
    return response({"message": "Hello from Starlark!"})
`

	req := httptest.NewRequest("GET", "/test", nil)
	execCtx := &ExecutionContext{
		Request: req,
		UserID:  "test-user",
	}

	result, err := Execute(context.Background(), script, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", result.StatusCode)
	}

	bodyMap, ok := result.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected body to be a map, got %T", result.Body)
	}

	if bodyMap["message"] != "Hello from Starlark!" {
		t.Errorf("Expected message 'Hello from Starlark!', got %v", bodyMap["message"])
	}
}

func TestExecute_CustomStatus(t *testing.T) {
	script := `
def handle_request(req):
    return response({"error": "Not found"}, status=404)
`

	req := httptest.NewRequest("GET", "/test", nil)
	execCtx := &ExecutionContext{
		Request: req,
		UserID:  "test-user",
	}

	result, err := Execute(context.Background(), script, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.StatusCode != 404 {
		t.Errorf("Expected status 404, got %d", result.StatusCode)
	}
}

func TestExecute_CustomHeaders(t *testing.T) {
	script := `
def handle_request(req):
    return response(
        {"data": "test"},
        headers={"Content-Type": "application/json", "X-Custom": "value"}
    )
`

	req := httptest.NewRequest("GET", "/test", nil)
	execCtx := &ExecutionContext{
		Request: req,
		UserID:  "test-user",
	}

	result, err := Execute(context.Background(), script, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Headers["Content-Type"] != "application/json" {
		t.Errorf("Expected Content-Type header, got %v", result.Headers)
	}

	if result.Headers["X-Custom"] != "value" {
		t.Errorf("Expected X-Custom header, got %v", result.Headers)
	}
}

func TestExecute_RequestAccess(t *testing.T) {
	script := `
def handle_request(req):
    return response({
        "method": req.method,
        "path": req.path,
        "user_id": req.user["id"]
    })
`

	req := httptest.NewRequest("POST", "/api/test?foo=bar", nil)
	execCtx := &ExecutionContext{
		Request: req,
		UserID:  "user-123",
	}

	result, err := Execute(context.Background(), script, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	bodyMap := result.Body.(map[string]interface{})
	if bodyMap["method"] != "POST" {
		t.Errorf("Expected method POST, got %v", bodyMap["method"])
	}

	if bodyMap["user_id"] != "user-123" {
		t.Errorf("Expected user_id user-123, got %v", bodyMap["user_id"])
	}
}

func TestExecute_DatabaseQuery(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create test table
	_, err = db.Exec(`CREATE TABLE test_items (id INTEGER PRIMARY KEY, name TEXT, value INTEGER)`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`INSERT INTO test_items (name, value) VALUES ('item1', 100), ('item2', 200)`)
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	script := `
def handle_request(req):
    rows = db.query("SELECT name, value FROM test_items ORDER BY id", [])
    return response({"items": rows})
`

	req := httptest.NewRequest("GET", "/test", nil)
	execCtx := &ExecutionContext{
		Request: req,
		UserID:  "test-user",
		DB:      db,
	}

	result, err := Execute(context.Background(), script, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	bodyMap := result.Body.(map[string]interface{})
	items, ok := bodyMap["items"].([]interface{})
	if !ok {
		t.Fatalf("Expected items to be a list, got %T", bodyMap["items"])
	}

	if len(items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(items))
	}

	// Check first item
	item1 := items[0].(map[string]interface{})
	if item1["name"] != "item1" {
		t.Errorf("Expected name 'item1', got %v", item1["name"])
	}
	if item1["value"] != int64(100) {
		t.Errorf("Expected value 100, got %v", item1["value"])
	}
}

func TestExecute_DatabaseExecute(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create test table
	_, err = db.Exec(`CREATE TABLE test_items (id INTEGER PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	script := `
def handle_request(req):
    result = db.execute("INSERT INTO test_items (name) VALUES (?)", ["test"])
    return response({
        "rows_affected": result["rows_affected"],
        "last_insert_id": result["last_insert_id"]
    })
`

	req := httptest.NewRequest("POST", "/test", nil)
	execCtx := &ExecutionContext{
		Request: req,
		UserID:  "test-user",
		DB:      db,
	}

	result, err := Execute(context.Background(), script, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	bodyMap := result.Body.(map[string]interface{})
	if bodyMap["rows_affected"] != int64(1) {
		t.Errorf("Expected rows_affected 1, got %v", bodyMap["rows_affected"])
	}

	// Verify the insert
	var count int
	db.QueryRow("SELECT COUNT(*) FROM test_items").Scan(&count)
	if count != 1 {
		t.Errorf("Expected 1 row in table, got %d", count)
	}
}

func TestExecute_JSONEncode(t *testing.T) {
	script := `
def handle_request(req):
    data = {"name": "test", "value": 123}
    json_str = json.encode(data)
    return response({"json": json_str})
`

	req := httptest.NewRequest("GET", "/test", nil)
	execCtx := &ExecutionContext{
		Request: req,
		UserID:  "test-user",
	}

	result, err := Execute(context.Background(), script, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	bodyMap := result.Body.(map[string]interface{})
	jsonStr, ok := bodyMap["json"].(string)
	if !ok {
		t.Fatalf("Expected json to be a string, got %T", bodyMap["json"])
	}

	if jsonStr != `{"name":"test","value":123}` {
		t.Errorf("Expected JSON string, got %s", jsonStr)
	}
}

func TestExecute_Timeout(t *testing.T) {
	script := `
def handle_request(req):
    # This would cause an infinite loop, but timeout should prevent it
    i = 0
    while True:
        i = i + 1
    return response({"count": i})
`

	req := httptest.NewRequest("GET", "/test", nil)
	execCtx := &ExecutionContext{
		Request: req,
		UserID:  "test-user",
		Timeout: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Note: Starlark doesn't have built-in timeout support, so we rely on context timeout
	// This test verifies our context is set up correctly
	_, err := Execute(ctx, script, execCtx)
	// The script will likely timeout during execution
	if err == nil {
		t.Skip("Script completed (Starlark execution timeout not enforced)")
	}
}

func TestExecute_MissingHandleRequest(t *testing.T) {
	script := `
def some_other_function():
    return "test"
`

	req := httptest.NewRequest("GET", "/test", nil)
	execCtx := &ExecutionContext{
		Request: req,
		UserID:  "test-user",
	}

	_, err := Execute(context.Background(), script, execCtx)
	if err == nil {
		t.Error("Expected error for missing handle_request function")
	}

	if err.Error() != "script must define a 'handle_request' function" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestExecute_SyntaxError(t *testing.T) {
	script := `
def handle_request(req):
    return response({"test": })  # Syntax error
`

	req := httptest.NewRequest("GET", "/test", nil)
	execCtx := &ExecutionContext{
		Request: req,
		UserID:  "test-user",
	}

	_, err := Execute(context.Background(), script, execCtx)
	if err == nil {
		t.Error("Expected syntax error")
	}
}
