package migrate

import (
	"strings"
	"testing"
)

// TestSplitSQLSkipsComments 确认 splitSQL 会跳过 SQL 行注释。
func TestSplitSQLSkipsComments(t *testing.T) {
	in := `
-- comment
CREATE TABLE IF NOT EXISTS t (id INT);
`
	stmts := splitSQL(in)
	if len(stmts) != 1 || !strings.Contains(stmts[0], "CREATE TABLE") {
		t.Fatalf("got %v", stmts)
	}
}
