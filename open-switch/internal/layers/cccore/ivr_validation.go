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
			Type           string            `json:"type"`
			Action         string            `json:"action"`
			File           string            `json:"file"`
			QueueID        string            `json:"queue_id"`
			Next           string            `json:"next"`
			Default        string            `json:"default"`
			Invalid        string            `json:"invalid"`
			Open           string            `json:"open"`
			Closed         string            `json:"closed"`
			Busy           string            `json:"busy"`
			Schedule       string            `json:"schedule"`
			WaitingGt      int               `json:"waiting_gt"`
			Timeout        int               `json:"timeout_sec"`
			Choices        map[string]string `json:"choices"`
			AcceptedDigits string            `json:"accepted_digits"`
			ResultKey      string            `json:"result_key"`
			MaxRetries     *int              `json:"max_retries"`
			SessionType    string            `json:"session_type"`
		} `json:"nodes"`
	}
	if json.Unmarshal([]byte(payload), &doc) != nil || doc.Start == "" || len(doc.Nodes) == 0 || len(doc.Nodes) > 100 {
		return errs.InvalidRequest("IVR 图无效")
	}
	edges := map[string][]string{}
	if _, ok := doc.Nodes[doc.Start]; !ok {
		return errs.InvalidRequest("IVR start 节点不存在")
	}
	for id, node := range doc.Nodes {
		if node.MaxRetries != nil && (*node.MaxRetries < 0 || *node.MaxRetries > 5) {
			return errs.InvalidRequest("IVR 无效按键重试必须为 0–5 次")
		}
		if node.Timeout < 0 || node.Timeout > 120 {
			return errs.InvalidRequest("IVR 超时必须为 0–120 秒")
		}
		switch node.Type {
		case "hangup":
		case "route_queue":
			if node.SessionType != "" && node.SessionType != "audio" && node.SessionType != "video" {
				return errs.InvalidRequest("IVR 通话类型无效")
			}
			if !queues[node.QueueID] {
				return errs.InvalidRequest("IVR 引用了不存在的队列")
			}
		case "play":
			if node.Next == "" {
				return errs.InvalidRequest("play 必须指定 next")
			}
			edges[id] = []string{node.Next}
		case "menu", "business_action":
			if len(node.Choices) == 0 {
				return errs.InvalidRequest("IVR 分支不能为空")
			}
			if node.Type == "business_action" && (strings.TrimSpace(node.Action) == "" || node.Timeout < 1) {
				return errs.InvalidRequest("业务动作必须指定 action 与超时")
			}
			if node.Default == "" {
				return errs.InvalidRequest("IVR 分支必须指定 default 超时去向")
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
		case "time_condition":
			schedule := strings.TrimSpace(node.Schedule)
			if schedule != "" && schedule != "always" {
				if err := validateHours(schedule); err != nil {
					return err
				}
			}
			if node.Open == "" || node.Closed == "" {
				return errs.InvalidRequest("时间判断必须配置营业与非营业去向")
			}
			edges[id] = []string{node.Open, node.Closed}
		case "queue_condition":
			if !queues[node.QueueID] {
				return errs.InvalidRequest("排队判断引用了不存在的队列")
			}
			if node.Open == "" {
				return errs.InvalidRequest("排队判断必须配置空闲去向")
			}
			if node.Busy == "" && node.Closed == "" {
				return errs.InvalidRequest("排队判断必须配置忙碌去向")
			}
			edges[id] = []string{node.Open, node.Busy, node.Closed}
		case "voicemail":
		case "collect_input":
			if strings.TrimSpace(node.ResultKey) == "" || node.AcceptedDigits == "" {
				return errs.InvalidRequest("按键采集必须指定 result_key 与 accepted_digits")
			}
			for _, digit := range node.AcceptedDigits {
				if !strings.ContainsRune("0123456789*#", digit) {
					return errs.InvalidRequest("按键采集包含无效 DTMF")
				}
			}
			if node.Next == "" || node.Default == "" {
				return errs.InvalidRequest("按键采集必须指定 next 与 default")
			}
			if node.Next != "" {
				edges[id] = []string{node.Next}
			}
			if node.Default != "" {
				if edges[id] == nil {
					edges[id] = []string{node.Default}
				} else {
					edges[id] = append(edges[id], node.Default)
				}
			}
		case "tts", "asr":
			if node.Default == "" {
				return errs.InvalidRequest("tts/asr 节点必须指定 default 兜底分支")
			}
			edges[id] = []string{node.Default}
		default:
			return errs.InvalidRequest("不支持的 IVR 节点: " + node.Type)
		}
		for _, target := range edges[id] {
			if target == "" {
				continue
			}
			if _, ok := doc.Nodes[target]; !ok {
				return errs.InvalidRequest("IVR 后续节点不存在")
			}
		}
	}
	seen := map[string]bool{doc.Start: true}
	queue := []string{doc.Start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, next := range edges[id] {
			if next == "" {
				continue
			}
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	for id := range doc.Nodes {
		if !seen[id] {
			return errs.InvalidRequest("IVR 存在不可达节点")
		}
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
