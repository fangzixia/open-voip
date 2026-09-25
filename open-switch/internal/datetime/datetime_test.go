package datetime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDateTimeRoundTrip(t *testing.T) {
	timestamp := time.Date(2026, 9, 25, 18, 12, 35, 987654321, time.FixedZone("HKT", 8*3600))
	type payload struct {
		At     *DateTime `json:"at"`
		Nested []struct {
			At DateTime `json:"at"`
		} `json:"nested"`
		Note string `json:"note"`
	}
	in := payload{At: &DateTime{Time: timestamp}, Nested: []struct {
		At DateTime `json:"at"`
	}{{At: DateTime{Time: timestamp}}}, Note: "2026-09-25T18:12:35+08:00"}
	raw, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw); got != `{"at":"2026-09-25 10:12:35","nested":[{"at":"2026-09-25 10:12:35"}],"note":"2026-09-25T18:12:35+08:00"}` {
		t.Fatalf("wire format = %s", got)
	}
	var out payload
	if err := Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.At == nil || !out.At.Time.Equal(timestamp.Truncate(time.Second)) || !out.Nested[0].At.Time.Equal(timestamp.Truncate(time.Second)) {
		t.Fatalf("round trip = %+v", out)
	}
	if _, err := Parse("2026-09-25T10:12:35Z"); err == nil {
		t.Fatal("accepted wrong format")
	}
	if _, err := Parse("2026-09-25 10:12:35"); err != nil {
		t.Fatal(err)
	}
}

func TestNestedTimesAndDatesWithoutGeneratedMethods(t *testing.T) {
	stamp := time.Date(2026, 9, 25, 18, 12, 35, 0, time.FixedZone("HKT", 8*3600))
	type payload struct {
		At     time.Time              `json:"at"`
		Until  *time.Time             `json:"until,omitempty"`
		Day    time.Time              `json:"day" datetime:"date"`
		Date   Date                   `json:"date"`
		Nested map[string][]time.Time `json:"nested"`
	}
	in := payload{At: stamp, Day: stamp, Date: Date{Time: stamp}, Nested: map[string][]time.Time{"events": {stamp}}}
	raw, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"at":"2026-09-25 10:12:35","date":"2026-09-25","day":"2026-09-25","nested":{"events":["2026-09-25 10:12:35"]}}`
	if string(raw) != want {
		t.Fatalf("wire format = %s", raw)
	}
	var out payload
	if err := Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if !out.At.Equal(stamp) || !out.Day.Equal(time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)) || out.Until != nil || !out.Nested["events"][0].Equal(stamp) {
		t.Fatalf("round trip = %+v", out)
	}
	if direct, err := json.Marshal(Date{Time: stamp}); err != nil || string(direct) != `"2026-09-25"` {
		t.Fatalf("date direct JSON = %s, %v", direct, err)
	}
	localMidnight := time.Date(2026, 9, 25, 0, 15, 0, 0, time.FixedZone("HKT", 8*3600))
	if FormatDate(localMidnight) != "2026-09-25" {
		t.Fatal("date changed when converting its time zone")
	}
	var directDate Date
	if err := json.Unmarshal([]byte(`"2026-09-25"`), &directDate); err != nil || FormatDate(directDate.Time) != "2026-09-25" {
		t.Fatalf("date direct decode = %+v, %v", directDate, err)
	}
	if _, err := ParseDate("2026-02-30"); err == nil {
		t.Fatal("accepted invalid calendar date")
	}
	if err := Unmarshal([]byte(`{"day":"2026-09-25 10:12:35"}`), &out); err == nil {
		t.Fatal("accepted timestamp in date field")
	}
	if err := UnmarshalStrict([]byte(`{"extra":1}`), &out); err == nil {
		t.Fatal("accepted unknown field")
	}
	if err := UnmarshalStrict([]byte(`{} {}`), &out); err == nil {
		t.Fatal("accepted multiple JSON values")
	}
	if err := UnmarshalStrict([]byte(`{"at":"2026-09-25T10:12:35Z"}`), &out); err == nil {
		t.Fatal("accepted RFC3339 in current API input")
	}
	var legacy struct {
		At time.Time `json:"at"`
	}
	if err := Unmarshal([]byte(`{"at":"2026-09-25T10:12:35Z"}`), &legacy); err != nil {
		t.Fatalf("legacy peer input: %v", err)
	}
}

func TestBareTimeInMapUsesUnifiedFormat(t *testing.T) {
	stamp := time.Date(2026, 9, 25, 10, 12, 35, 0, time.UTC)
	raw, err := Marshal(map[string]any{"at": stamp})
	if err != nil || string(raw) != `{"at":"2026-09-25 10:12:35"}` {
		t.Fatalf("map timestamp = %s, %v", raw, err)
	}
}
