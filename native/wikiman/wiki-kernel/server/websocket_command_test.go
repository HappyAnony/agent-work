package server

import "testing"

func TestWebSocketCommandEnvelopeValidation(t *testing.T) {
	for _, field := range []string{"cmd", "reqId", "param", "pushMode"} {
		t.Run(field, func(t *testing.T) {
			request := map[string]interface{}{"cmd": "ping", "reqId": float64(1), "param": map[string]interface{}{}}
			if _, _, _, ok := parseWebSocketCommand(request); !ok {
				t.Fatal("valid command rejected")
			}
			if field == "pushMode" {
				request["param"] = map[string]interface{}{"pushMode": "invalid"}
			} else {
				request[field] = []interface{}{}
			}
			if _, _, _, ok := parseWebSocketCommand(request); ok {
				t.Fatal("malformed command admitted")
			}
		})
	}
}
