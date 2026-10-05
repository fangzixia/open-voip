package control

import (
	"testing"
	"time"
)

func TestWithinHoursFromSchedule(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	schedule := `{"timezone":"Asia/Shanghai","mon":"09:00-18:00","tue":"closed"}`
	open := time.Date(2026, 10, 5, 10, 0, 0, 0, loc) // Monday
	if !withinHoursFromSchedule(schedule, open) {
		t.Fatal("expected open during mon window")
	}
	closedDay := time.Date(2026, 10, 6, 10, 0, 0, 0, loc) // Tuesday closed
	if withinHoursFromSchedule(schedule, closedDay) {
		t.Fatal("expected closed on tuesday")
	}
	if !withinHoursFromSchedule("always", closedDay) {
		t.Fatal("always should be open")
	}
}
