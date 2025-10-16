package authz

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// QueryType represents the type of SQL operation
type QueryType string

const (
	QueryTypeSelect QueryType = "SELECT"
	QueryTypeInsert QueryType = "INSERT"
	QueryTypeUpdate QueryType = "UPDATE"
	QueryTypeDelete QueryType = "DELETE"
	QueryTypeUnknown QueryType = "UNKNOWN"
)

// ParsedQuery represents a parsed SQL query
type ParsedQuery struct {
	Type      QueryType
	TableName string
	RawSQL    string
}

// ParseQuery parses a SQL query to extract type and table name
// This is a simple parser - production use might want a full SQL parser
func ParseQuery(sql string) (*ParsedQuery, error) {
	// Normalize whitespace and convert to uppercase for parsing
	normalized := strings.TrimSpace(sql)
	upper := strings.ToUpper(normalized)

	pq := &ParsedQuery{
		RawSQL: normalized,
	}

	// Detect query type
	if strings.HasPrefix(upper, "SELECT") {
		pq.Type = QueryTypeSelect
		pq.TableName = extractTableFromSelect(normalized)
	} else if strings.HasPrefix(upper, "INSERT") {
		pq.Type = QueryTypeInsert
		pq.TableName = extractTableFromInsert(normalized)
	} else if strings.HasPrefix(upper, "UPDATE") {
		pq.Type = QueryTypeUpdate
		pq.TableName = extractTableFromUpdate(normalized)
	} else if strings.HasPrefix(upper, "DELETE") {
		pq.Type = QueryTypeDelete
		pq.TableName = extractTableFromDelete(normalized)
	} else {
		pq.Type = QueryTypeUnknown
	}

	if pq.TableName == "" && pq.Type != QueryTypeUnknown {
		return nil, fmt.Errorf("could not extract table name from query")
	}

	return pq, nil
}

// extractTableFromSelect extracts table name from SELECT query
// Handles: SELECT ... FROM table_name
func extractTableFromSelect(sql string) string {
	// Simple regex to match FROM table_name
	// This won't handle complex queries with joins, subqueries, etc.
	re := regexp.MustCompile(`(?i)FROM\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// extractTableFromInsert extracts table name from INSERT query
// Handles: INSERT INTO table_name
func extractTableFromInsert(sql string) string {
	re := regexp.MustCompile(`(?i)INSERT\s+INTO\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// extractTableFromUpdate extracts table name from UPDATE query
// Handles: UPDATE table_name SET ...
func extractTableFromUpdate(sql string) string {
	re := regexp.MustCompile(`(?i)UPDATE\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// extractTableFromDelete extracts table name from DELETE query
// Handles: DELETE FROM table_name
func extractTableFromDelete(sql string) string {
	re := regexp.MustCompile(`(?i)DELETE\s+FROM\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// ValidateQuery checks if a user has permission to execute a query
func ValidateQuery(db *sql.DB, userID, role, sqlQuery string) error {
	parsed, err := ParseQuery(sqlQuery)
	if err != nil {
		return fmt.Errorf("failed to parse query: %w", err)
	}

	if parsed.Type == QueryTypeUnknown {
		return fmt.Errorf("unsupported query type")
	}

	tableName := parsed.TableName

	// Check appropriate permission based on query type
	switch parsed.Type {
	case QueryTypeSelect:
		canRead, err := CanRead(db, userID, role, tableName)
		if err != nil {
			return fmt.Errorf("failed to check read permission: %w", err)
		}
		if !canRead {
			return fmt.Errorf("permission denied: cannot read from table %s", tableName)
		}

	case QueryTypeInsert, QueryTypeUpdate:
		canWrite, err := CanWrite(db, userID, role, tableName)
		if err != nil {
			return fmt.Errorf("failed to check write permission: %w", err)
		}
		if !canWrite {
			return fmt.Errorf("permission denied: cannot write to table %s", tableName)
		}

	case QueryTypeDelete:
		canDelete, err := CanDelete(db, userID, role, tableName)
		if err != nil {
			return fmt.Errorf("failed to check delete permission: %w", err)
		}
		if !canDelete {
			return fmt.Errorf("permission denied: cannot delete from table %s", tableName)
		}
	}

	return nil
}

// ApplyRowPolicies applies row-level security policies to a query
// This rewrites the query to add WHERE conditions from policies
func ApplyRowPolicies(sqlQuery, userID string, policies []RowPolicy) (string, error) {
	if len(policies) == 0 {
		return sqlQuery, nil
	}

	parsed, err := ParseQuery(sqlQuery)
	if err != nil {
		return "", fmt.Errorf("failed to parse query: %w", err)
	}

	// Only apply policies to SELECT, UPDATE, DELETE (not INSERT)
	if parsed.Type != QueryTypeSelect && parsed.Type != QueryTypeUpdate && parsed.Type != QueryTypeDelete {
		return sqlQuery, nil
	}

	// Build combined policy condition
	var conditions []string
	for _, policy := range policies {
		// Replace $user_id with actual user ID
		condition := strings.ReplaceAll(policy.SQLCondition, "$user_id", fmt.Sprintf("'%s'", userID))
		conditions = append(conditions, fmt.Sprintf("(%s)", condition))
	}

	if len(conditions) == 0 {
		return sqlQuery, nil
	}

	// Combine all conditions with AND
	combinedCondition := strings.Join(conditions, " AND ")

	// Check if query already has WHERE clause
	upperSQL := strings.ToUpper(sqlQuery)
	whereIdx := strings.Index(upperSQL, " WHERE ")

	if whereIdx != -1 {
		// Query has WHERE - append our condition with AND
		// Find position after WHERE
		insertPos := whereIdx + len(" WHERE ")
		rewritten := sqlQuery[:insertPos] + "(" + combinedCondition + ") AND (" + sqlQuery[insertPos:] + ")"
		return rewritten, nil
	}

	// No WHERE clause - add one
	// Need to find where to insert (before ORDER BY, LIMIT, etc.)
	insertPos := len(sqlQuery)

	// Check for ORDER BY, LIMIT, GROUP BY, etc.
	for _, keyword := range []string{" ORDER BY ", " LIMIT ", " GROUP BY ", " HAVING "} {
		if idx := strings.Index(upperSQL, keyword); idx != -1 && idx < insertPos {
			insertPos = idx
		}
	}

	rewritten := sqlQuery[:insertPos] + " WHERE " + combinedCondition + " " + sqlQuery[insertPos:]
	return strings.TrimSpace(rewritten), nil
}

// ValidateAndRewriteQuery validates permissions and applies row policies
func ValidateAndRewriteQuery(db *sql.DB, userID, role, sqlQuery string) (string, error) {
	// First validate permissions
	if err := ValidateQuery(db, userID, role, sqlQuery); err != nil {
		return "", err
	}

	// Parse query to get table and type
	parsed, err := ParseQuery(sqlQuery)
	if err != nil {
		return "", fmt.Errorf("failed to parse query: %w", err)
	}

	// Owner and Admin bypass row policies
	if role == RoleOwner || role == RoleAdmin {
		return sqlQuery, nil
	}

	// Get row policies for this table and user
	var policyType string
	switch parsed.Type {
	case QueryTypeSelect:
		policyType = "read"
	case QueryTypeUpdate:
		policyType = "write"
	case QueryTypeDelete:
		policyType = "delete"
	default:
		// INSERT doesn't use row policies
		return sqlQuery, nil
	}

	policies, err := GetRowPolicies(db, userID, parsed.TableName, policyType)
	if err != nil {
		return "", fmt.Errorf("failed to get row policies: %w", err)
	}

	// Apply row policies
	rewritten, err := ApplyRowPolicies(sqlQuery, userID, policies)
	if err != nil {
		return "", fmt.Errorf("failed to apply row policies: %w", err)
	}

	return rewritten, nil
}
