package control

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// withinHoursFromSchedule 按 IVR 节点内嵌 schedule JSON 判断是否在服务时间。
func withinHoursFromSchedule(schedule string, now time.Time) bool {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" || schedule == "always" {
		return true
	}
	var m map[string]string
	if json.Unmarshal([]byte(schedule), &m) != nil {
		return false
	}
	tz := m["timezone"]
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			now = now.In(loc)
		}
	}
	key := strings.ToLower(now.Weekday().String()[:3])
	win, ok := m[key]
	if !ok {
		win = m[strconv.Itoa(int(now.Weekday()))]
	}
	if holiday, exists := m[now.Format("2006-01-02")]; exists {
		win = holiday
	}
	if win == "" || win == "closed" {
		return false
	}
	parts := strings.Split(win, "-")
	if len(parts) != 2 {
		return false
	}
	cur := now.Format("15:04")
	return cur >= parts[0] && cur < parts[1]
}
