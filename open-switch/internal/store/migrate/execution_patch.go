package migrate

import (
	"fmt"
	"strings"
)

// Published source checksums remain immutable. These execution repairs apply
// only to the exact, reviewed broken historical bytes, and are persisted
// separately from their source checksum. Already applied versions are skipped.
func executionSQL(name, content string) (string, string, error) {
	sql := strings.TrimPrefix(content, "\ufeff")
	switch name {
	case "000001_baseline.sql":
		if checksumSQL(content) != "b966a58898a1bfcebad83a2e0445c9241cd6721768af2b212f7e20baef347ea4" {
			return "", "", fmt.Errorf("unreviewed historical baseline bytes")
		}
		fixed, e := repairBaselineComments(sql, 72)
		return fixed, "baseline_comment_quotes_v1", e
	case "000004_agent_uuid.sql":
		if checksumSQL(content) != "eb88c7f34c861fef52b47363f0a3b355fdbaf19ae7c28e45fcd494be24c05897" {
			return "", "", fmt.Errorf("unreviewed historical UUID migration bytes")
		}
		for _, column := range []string{"agent_id", "offered_agent", "last_agent_id"} {
			sql = strings.ReplaceAll(sql, "WHERE "+column+" = ''", "WHERE "+column+"::text = ''")
		}
		return sql, "uuid_empty_text_comparison_v1", nil
	}
	return sql, "", nil
}

func repairBaselineComments(content string, expected int) (string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	count := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "COMMENT ON ") && strings.Count(line, "'")%2 != 0 {
			if !strings.HasSuffix(line, ";") {
				return "", fmt.Errorf("unexpected damaged comment boundary")
			}
			lines[i] = strings.TrimSuffix(line, ";") + "';"
			count++
		}
	}
	if count != expected {
		return "", fmt.Errorf("historical comment patch count %d, expected %d", count, expected)
	}
	return strings.Join(lines, "\n"), nil
}
