package ivr

import (
	"encoding/json"
	"testing"
)

func TestCompileBusinessSurveyToGenericInput(t *testing.T) {
	raw, err := CompilePayload(`{"start":"rating","nodes":{"rating":{"type":"csat","file":"prompt.wav","timeout_sec":9,"next":"end","default":"end"},"end":{"type":"hangup"}},"layout":{"rating":{"x":123,"y":234}}}`)
	if err != nil {
		t.Fatal(err)
	}
	var doc Doc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	n := doc.Nodes["rating"]
	if n.Type != "collect_input" || n.ResultKey != "csat" || n.AcceptedDigits != "12345" || n.File != "prompt.wav" || n.Next != "end" || n.Default != "end" || n.TimeoutSec != 9 {
		t.Fatalf("compiled node: %+v", n)
	}
	if doc.Layout["rating"].X != 123 {
		t.Fatal("layout lost")
	}
}

func TestCompilePreservesGenericInputs(t *testing.T) {
	raw, err := CompilePayload(`{"start":"input","nodes":{"input":{"type":"collect_input","accepted_digits":"09*#","result_key":"reference","next":"end","default":"end"},"end":{"type":"hangup"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	var doc Doc
	_ = json.Unmarshal([]byte(raw), &doc)
	if n := doc.Nodes["input"]; n.ResultKey != "reference" || n.AcceptedDigits != "09*#" {
		t.Fatalf("generic input altered: %+v", n)
	}
}

func TestSurveyScoreOnlyInterpretsDeclaredBusinessResults(t *testing.T) {
	for _, c := range []struct {
		key, input string
		score      int
		valid      bool
	}{
		{"csat", "1", 1, true}, {"csat", "5", 5, true}, {"reference", "4", 0, false},
		{"csat", "9", 0, false}, {"csat", "04", 0, false}, {"csat", "", 0, false}, {"csat", "#", 0, false},
	} {
		score, valid := SurveyScore(c.key, c.input)
		if score != c.score || valid != c.valid {
			t.Fatalf("%+v: %d %v", c, score, valid)
		}
	}
}
