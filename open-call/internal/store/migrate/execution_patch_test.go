package migrate

import (
	"strings"
	"testing"
)

func TestHistoricalExecutionPatchIsExplicitAndFrozen(t *testing.T) {
	name := "000001_baseline.sql"
	raw, e := sqlFiles.ReadFile("sql/" + name)
	if e != nil {
		t.Fatal(e)
	}
	before := checksumSQL(string(raw))
	fixed, patch, e := executionSQL(name, string(raw))
	if e != nil || patch == "" {
		t.Fatalf("historical repair: %s %v", patch, e)
	}
	if checksumSQL(string(raw)) != before || checksumSQL(fixed) == before {
		t.Fatal("source history rewritten or repair absent")
	}
	for _, stmt := range splitSQL(fixed) {
		if strings.HasPrefix(stmt, "COMMENT ON ") && strings.Count(stmt, "'")%2 != 0 {
			t.Fatal("unclosed repaired comment")
		}
	}
	if _, _, e = executionSQL(name, string(raw)+"\n"); e == nil {
		t.Fatal("silently repaired unreviewed historical bytes")
	}
	untouched := "CREATE TABLE future (id INT);"
	if sql, patch, e := executionSQL("009999_future.sql", untouched); e != nil || patch != "" || sql != untouched {
		t.Fatal("repair leaked into future migrations")
	}
}
