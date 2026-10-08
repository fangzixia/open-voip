package ivr

import (
	"encoding/json"

	"open-call/internal/errs"
)

// CompilePayload 将业务节点编译为 Switch 通用交互节点；图与媒体能力由 Switch 校验。
func CompilePayload(raw string) (string, error) {
	var doc Doc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", errs.InvalidRequest("草稿 JSON 无效")
	}
	for id, node := range doc.Nodes {
		if node.Type == "csat" {
			node.Type = "collect_input"
			node.AcceptedDigits = "12345"
			node.ResultKey = "csat"
			doc.Nodes[id] = node
		}
	}
	payload, err := json.Marshal(doc)
	return string(payload), err
}

// SurveyScore 在业务侧解释评价输入，其他采集结果不作为满意度处理。
func SurveyScore(resultKey, input string) (int, bool) {
	if resultKey != "csat" || len(input) != 1 || input[0] < '1' || input[0] > '5' {
		return 0, false
	}
	return int(input[0] - '0'), true
}
