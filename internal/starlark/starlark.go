package starlark

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// ExecutionContext holds the context for executing a Starlark script
type ExecutionContext struct {
	DB      *sql.DB
	UserID  string
	Request *http.Request
	Timeout time.Duration
}

// ExecutionResult holds the result of executing a Starlark script
type ExecutionResult struct {
	StatusCode int
	Headers    map[string]string
	Body       interface{}
}

// Execute runs a Starlark script with the given context and returns the result
func Execute(ctx context.Context, script string, execCtx *ExecutionContext) (*ExecutionResult, error) {
	// Set timeout for execution
	if execCtx.Timeout == 0 {
		execCtx.Timeout = 5 * time.Second
	}

	// Create context with timeout
	execCtx2, cancel := context.WithTimeout(ctx, execCtx.Timeout)
	defer cancel()

	// Create thread with execution context
	thread := &starlark.Thread{
		Name: "wce-script",
	}

	// Build predeclared environment with safe builtins only
	predeclared := buildPredeclared(execCtx2, execCtx)

	// Execute the script
	globals, err := starlark.ExecFile(thread, "script.star", script, predeclared)
	if err != nil {
		return nil, fmt.Errorf("script execution error: %w", err)
	}

	// Check if the script returned a handle_request function
	handleRequestVal, ok := globals["handle_request"]
	if !ok {
		return nil, fmt.Errorf("script must define a 'handle_request' function")
	}

	handleRequest, ok := handleRequestVal.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("handle_request must be a function")
	}

	// Call handle_request with request context
	requestObj := buildRequestObject(execCtx)
	args := starlark.Tuple{requestObj}
	result, err := starlark.Call(thread, handleRequest, args, nil)
	if err != nil {
		return nil, fmt.Errorf("handle_request error: %w", err)
	}

	// Parse the result
	return parseResult(result)
}

// buildPredeclared creates the predeclared environment for Starlark scripts
func buildPredeclared(ctx context.Context, execCtx *ExecutionContext) starlark.StringDict {
	return starlark.StringDict{
		// Provide db module for database access
		"db": starlarkstruct.FromStringDict(starlark.String("db"), starlark.StringDict{
			"query":   starlark.NewBuiltin("db.query", makeQueryFunc(ctx, execCtx)),
			"execute": starlark.NewBuiltin("db.execute", makeExecuteFunc(ctx, execCtx)),
		}),
		// Standard built-ins (limited for safety)
		"json": starlarkstruct.FromStringDict(starlark.String("json"), starlark.StringDict{
			"encode": starlark.NewBuiltin("json.encode", jsonEncode),
			"decode": starlark.NewBuiltin("json.decode", jsonDecode),
		}),
		// Response builder
		"response": starlark.NewBuiltin("response", makeResponseFunc()),
	}
}

// buildRequestObject creates a Starlark object representing the HTTP request
func buildRequestObject(execCtx *ExecutionContext) *starlarkstruct.Struct {
	req := execCtx.Request

	// Parse query parameters
	queryParams := starlark.NewDict(len(req.URL.Query()))
	for key, values := range req.URL.Query() {
		if len(values) > 0 {
			queryParams.SetKey(starlark.String(key), starlark.String(values[0]))
		}
	}

	// Parse headers
	headers := starlark.NewDict(len(req.Header))
	for key, values := range req.Header {
		if len(values) > 0 {
			headers.SetKey(starlark.String(key), starlark.String(values[0]))
		}
	}

	// Create user object
	userDict := starlark.NewDict(1)
	userDict.SetKey(starlark.String("id"), starlark.String(execCtx.UserID))

	return starlarkstruct.FromStringDict(starlark.String("request"), starlark.StringDict{
		"method":  starlark.String(req.Method),
		"path":    starlark.String(req.URL.Path),
		"query":   queryParams,
		"headers": headers,
		"user":    userDict,
	})
}

// makeQueryFunc creates the db.query function
func makeQueryFunc(ctx context.Context, execCtx *ExecutionContext) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		// Parse arguments: db.query(sql, params)
		var sqlStr string
		var paramsVal starlark.Value

		if err := starlark.UnpackArgs("db.query", args, kwargs, "sql", &sqlStr, "params?", &paramsVal); err != nil {
			return nil, err
		}

		// Convert params to Go values
		params := []interface{}{}
		if paramsVal != nil {
			if list, ok := paramsVal.(*starlark.List); ok {
				for i := 0; i < list.Len(); i++ {
					val := list.Index(i)
					params = append(params, starlarkToGo(val))
				}
			}
		}

		// Execute query
		rows, err := execCtx.DB.QueryContext(ctx, sqlStr, params...)
		if err != nil {
			return nil, fmt.Errorf("query error: %w", err)
		}
		defer rows.Close()

		// Get column names
		columns, err := rows.Columns()
		if err != nil {
			return nil, err
		}

		// Build result list
		result := starlark.NewList([]starlark.Value{})
		for rows.Next() {
			// Create slice to hold values
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, err
			}

			// Build row dict
			rowDict := starlark.NewDict(len(columns))
			for i, col := range columns {
				rowDict.SetKey(starlark.String(col), goToStarlark(values[i]))
			}
			result.Append(rowDict)
		}

		return result, nil
	}
}

// makeExecuteFunc creates the db.execute function
func makeExecuteFunc(ctx context.Context, execCtx *ExecutionContext) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		// Parse arguments: db.execute(sql, params)
		var sqlStr string
		var paramsVal starlark.Value

		if err := starlark.UnpackArgs("db.execute", args, kwargs, "sql", &sqlStr, "params?", &paramsVal); err != nil {
			return nil, err
		}

		// Convert params to Go values
		params := []interface{}{}
		if paramsVal != nil {
			if list, ok := paramsVal.(*starlark.List); ok {
				for i := 0; i < list.Len(); i++ {
					val := list.Index(i)
					params = append(params, starlarkToGo(val))
				}
			}
		}

		// Execute statement
		result, err := execCtx.DB.ExecContext(ctx, sqlStr, params...)
		if err != nil {
			return nil, fmt.Errorf("execute error: %w", err)
		}

		// Get rows affected
		rowsAffected, _ := result.RowsAffected()
		lastInsertID, _ := result.LastInsertId()

		// Return result dict
		resultDict := starlark.NewDict(2)
		resultDict.SetKey(starlark.String("rows_affected"), starlark.MakeInt64(rowsAffected))
		resultDict.SetKey(starlark.String("last_insert_id"), starlark.MakeInt64(lastInsertID))

		return resultDict, nil
	}
}

// makeResponseFunc creates the response builder function
func makeResponseFunc() func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		// Parse arguments: response(body, status=200, headers={})
		var bodyVal starlark.Value
		var statusVal starlark.Value
		var headersVal starlark.Value

		if err := starlark.UnpackArgs("response", args, kwargs, "body", &bodyVal, "status?", &statusVal, "headers?", &headersVal); err != nil {
			return nil, err
		}

		// Build response dict
		responseDict := starlark.NewDict(3)
		responseDict.SetKey(starlark.String("body"), bodyVal)

		if statusVal != nil {
			responseDict.SetKey(starlark.String("status"), statusVal)
		} else {
			responseDict.SetKey(starlark.String("status"), starlark.MakeInt(200))
		}

		if headersVal != nil {
			responseDict.SetKey(starlark.String("headers"), headersVal)
		} else {
			responseDict.SetKey(starlark.String("headers"), starlark.NewDict(0))
		}

		return responseDict, nil
	}
}

// jsonEncode encodes a Starlark value to JSON
func jsonEncode(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var val starlark.Value
	if err := starlark.UnpackArgs("json.encode", args, kwargs, "value", &val); err != nil {
		return nil, err
	}

	goVal := starlarkToGo(val)
	jsonBytes, err := json.Marshal(goVal)
	if err != nil {
		return nil, err
	}

	return starlark.String(string(jsonBytes)), nil
}

// jsonDecode decodes JSON to a Starlark value
func jsonDecode(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var jsonStr string
	if err := starlark.UnpackArgs("json.decode", args, kwargs, "json", &jsonStr); err != nil {
		return nil, err
	}

	var goVal interface{}
	if err := json.Unmarshal([]byte(jsonStr), &goVal); err != nil {
		return nil, err
	}

	return goToStarlark(goVal), nil
}

// parseResult converts a Starlark value to an ExecutionResult
func parseResult(val starlark.Value) (*ExecutionResult, error) {
	dict, ok := val.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("handle_request must return a dict (response object)")
	}

	result := &ExecutionResult{
		StatusCode: 200,
		Headers:    make(map[string]string),
	}

	// Get status
	if statusVal, found, _ := dict.Get(starlark.String("status")); found {
		if statusInt, ok := statusVal.(starlark.Int); ok {
			status, _ := statusInt.Int64()
			result.StatusCode = int(status)
		}
	}

	// Get headers
	if headersVal, found, _ := dict.Get(starlark.String("headers")); found {
		if headersDict, ok := headersVal.(*starlark.Dict); ok {
			for _, item := range headersDict.Items() {
				key := item[0].(starlark.String).GoString()
				value := item[1].(starlark.String).GoString()
				result.Headers[key] = value
			}
		}
	}

	// Get body
	if bodyVal, found, _ := dict.Get(starlark.String("body")); found {
		result.Body = starlarkToGo(bodyVal)
	}

	return result, nil
}

// starlarkToGo converts a Starlark value to a Go value
func starlarkToGo(val starlark.Value) interface{} {
	switch v := val.(type) {
	case starlark.NoneType:
		return nil
	case starlark.Bool:
		return bool(v)
	case starlark.Int:
		i, _ := v.Int64()
		return i
	case starlark.Float:
		return float64(v)
	case starlark.String:
		return v.GoString()
	case *starlark.List:
		result := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			result[i] = starlarkToGo(v.Index(i))
		}
		return result
	case *starlark.Dict:
		result := make(map[string]interface{})
		for _, item := range v.Items() {
			key := starlarkToGo(item[0])
			value := starlarkToGo(item[1])
			if keyStr, ok := key.(string); ok {
				result[keyStr] = value
			}
		}
		return result
	default:
		return v.String()
	}
}

// goToStarlark converts a Go value to a Starlark value
func goToStarlark(val interface{}) starlark.Value {
	if val == nil {
		return starlark.None
	}

	switch v := val.(type) {
	case bool:
		return starlark.Bool(v)
	case int:
		return starlark.MakeInt(v)
	case int64:
		return starlark.MakeInt64(v)
	case float64:
		return starlark.Float(v)
	case string:
		return starlark.String(v)
	case []byte:
		return starlark.String(string(v))
	case []interface{}:
		list := make([]starlark.Value, len(v))
		for i, item := range v {
			list[i] = goToStarlark(item)
		}
		return starlark.NewList(list)
	case map[string]interface{}:
		dict := starlark.NewDict(len(v))
		for key, value := range v {
			dict.SetKey(starlark.String(key), goToStarlark(value))
		}
		return dict
	default:
		return starlark.String(fmt.Sprint(v))
	}
}
