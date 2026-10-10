package migrate

import "testing"

func TestSplitSQLBOM(t *testing.T) {
	original := "\ufeff-- migration\nSELECT 1;"
	statements := splitSQL(original)
	if len(statements) != 1 || statements[0] != "SELECT 1" {
		t.Fatalf("BOM entered SQL: %q", statements)
	}
	if checksumSQL(original) == checksumSQL(original[3:]) {
		t.Fatal("historical checksum was normalized")
	}
}
