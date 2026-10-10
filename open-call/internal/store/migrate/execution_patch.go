package migrate

import (
	"fmt"
	"strings"
)

// Keep published bytes and installed source checksums intact. The separately
// recorded execution repair is limited to this exact broken historical file.
func executionSQL(name, content string) (string, string, error) {
	sql := strings.TrimPrefix(content, "\ufeff")
	if name != "000001_baseline.sql" {
		return sql, "", nil
	}
	if checksumSQL(content) != "a9bcf2d8ac2a3ec999246a981437076cb2184d40d8236e3ef45687a9ecac9f55" {
		return "", "", fmt.Errorf("unreviewed historical baseline bytes")
	}
	lines := strings.Split(strings.ReplaceAll(sql, "\r\n", "\n"), "\n")
	comments, seeds := 0, 0
	for i, line := range lines {
		if strings.HasPrefix(line, "COMMENT ON ") && strings.Count(line, "'")%2 != 0 {
			if !strings.HasSuffix(line, ";") {
				return "", "", fmt.Errorf("unexpected damaged comment boundary")
			}
			lines[i] = strings.TrimSuffix(line, ";") + "';"
			comments++
		}
		// Only the four malformed seed rows in this frozen file are repaired.
		if strings.Count(line, "'")%2 != 0 {
			for prefix, replacement := range map[string]string{
				"('admin',":        "  ('admin', '管理员', TRUE),",
				"('skills.read',":  "  ('skills.read', '查看技能'),",
				"('skills.write',": "  ('skills.write', '管理技能'),",
				"('status.read',":  "  ('status.read', '查看运行状态');",
			} {
				if strings.HasPrefix(strings.TrimSpace(line), prefix) {
					lines[i] = replacement
					seeds++
				}
			}
		}
	}
	if comments != 50 || seeds != 4 {
		return "", "", fmt.Errorf("unexpected historical repair count: comments=%d seeds=%d", comments, seeds)
	}
	return strings.Join(lines, "\n"), "baseline_comment_and_seed_quotes_v1", nil
}
