package cccore

import (
	"encoding/json"
	"open-switch/internal/errs"
	"strings"
	"time"
)

// validateIVR 校验 IVR 图结构、分支、业务动作节点与可达性（发布前调用）。
func validateIVR(payload string, queues map[string]bool) error {
	var doc struct {
		Start string `json:"start"`
		Nodes map[string]struct {
			Type    string            `json:"type"`
			Action  string            `json:"action"`
			File    string            `json:"file"`
			QueueID string            `json:"queue_id"`
			Next    string            `json:"next"`
			Default string            `json:"default"`
			Invalid string            `json:"invalid"`
			Open    string            `json:"open"`
			Closed  string            `json:"closed"`
			Timeout int               `json:"timeout_sec"`
			Choices map[string]string `json:"choices"`
		} `json:"nodes"`
	}
	if json.Unmarshal([]byte(payload), &doc) != nil || doc.Start == "" || len(doc.Nodes) == 0 || len(doc.Nodes) > 100 {
		return errs.InvalidRequest("IVR 图无效")
	}
	edges := map[string][]string{}
	for id, node := range doc.Nodes {
		if node.Timeout < 0 || node.Timeout > 120 {
			return errs.InvalidRequest("IVR 超时必须为 0–120 秒")
		}
		switch node.Type {
		case "hangup":
		case "route_queue":
			if !queues[node.QueueID] {
				return errs.InvalidRequest("IVR 引用了不存在的队列")
			}
		case "play":
			edges[id] = []string{node.Next}
		case "menu", "business_action":
			if len(node.Choices) == 0 {
				return errs.InvalidRequest("IVR 分支不能为空")
			}
			if node.Type == "business_action" && (strings.TrimSpace(node.Action) == "" || node.Timeout < 1) {
				return errs.InvalidRequest("业务动作必须指定 action 与超时")
			}
			edges[id] = []string{node.Default}
			for key, target := range node.Choices {
				if node.Type == "menu" && (len(key) != 1 || !strings.Contains("0123456789*#", key)) {
					return errs.InvalidRequest("无效 DTMF 分支")
				}
				edges[id] = append(edges[id], target)
			}
			if node.Invalid != "" {
				edges[id] = append(edges[id], node.Invalid)
			}
		case "time_check":
			if !queues[node.QueueID] {
				return errs.InvalidRequest("工作时间节点引用不存在的队列")
			}
			edges[id] = []string{node.Open, node.Closed}
		case "tts", "asr":
			if node.Default == "" {
				return errs.InvalidRequest("tts/asr 节点必须指定 default 兜底分支")
			}
			edges[id] = []string{node.Default}
		default:
			return errs.InvalidRequest("不支持的 IVR 节点: " + node.Type)
		}
		for _, target := range edges[id] {
			if _, ok := doc.Nodes[target]; !ok {
				return errs.InvalidRequest("IVR 后续节点不存在")
			}
		}
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return false
		}
		if done[id] {
			return true
		}
		if _, ok := doc.Nodes[id]; !ok {
			return false
		}
		visiting[id] = true
		for _, next := range edges[id] {
			if !visit(next) {
				return false
			}
		}
		visiting[id] = false
		done[id] = true
		return true
	}
	if !visit(doc.Start) || len(done) != len(doc.Nodes) {
		return errs.InvalidRequest("IVR 包含循环或不可达节点")
	}
	return nil
}

// validateHours 校验队列工作时间 JSON（时区、星期与 HH:MM 窗口）。
func validateHours(raw string) error {
	if raw == "always" {
		return nil
	}
	var hours map[string]string
	if json.Unmarshal([]byte(raw), &hours) != nil {
		return errs.InvalidRequest("工作时间必须为 weekday → HH:MM-HH:MM 的映射")
	}
	days := map[string]bool{"mon": true, "tue": true, "wed": true, "thu": true, "fri": true, "sat": true, "sun": true, "0": true, "1": true, "2": true, "3": true, "4": true, "5": true, "6": true}
	for day, window := range hours {
		if day == "timezone" {
			if _, err := time.LoadLocation(window); err != nil {
				return errs.InvalidRequest("工作时间时区无效")
			}
			continue
		}
		if !days[day] {
			if _, err := time.Parse("2006-01-02", day); err != nil {
				return errs.InvalidRequest("工作时间日期无效")
			}
		}
		if window == "closed" {
			continue
		}
		parts := strings.Split(window, "-")
		if len(parts) != 2 {
			return errs.InvalidRequest("工作时间窗口无效")
		}
		for _, part := range parts {
			if _, err := time.Parse("15:04", part); err != nil {
				return errs.InvalidRequest("工作时间须为 HH:MM")
			}
		}
		if parts[0] >= parts[1] {
			return errs.InvalidRequest("工作时间结束须晚于开始；跨日请拆分")
		}
	}
	return nil
}
