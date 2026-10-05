package cccore

import "testing"

func TestValidateIVRTimeAndQueueCondition(t *testing.T) {
	payload := `{"start":"a","nodes":{
		"a":{"type":"time_condition","schedule":"always","open":"b","closed":"c"},
		"b":{"type":"queue_condition","queue_id":"q1","open":"d","busy":"c"},
		"c":{"type":"hangup"},
		"d":{"type":"route_queue","queue_id":"q1"}
	}}`
	queues := map[string]bool{"q1": true}
	if err := validateIVR(payload, queues); err != nil {
		t.Fatal(err)
	}
}
