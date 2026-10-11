package mcprpc

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestInitializeInstructionsAreOptionalAndPreserveProtocol(t *testing.T) {
	for _, guidance := range []string{"", "Archive stage outputs"} {
		w := httptest.NewRecorder()
		r := Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize", Params: json.RawMessage(`{"protocolVersion":"2025-03-26"}`)}
		if !CommonWithInstructions(w, r, "studio", "1", guidance) {
			t.Fatal("not handled")
		}
		var reply struct {
			Result map[string]any `json:"result"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
			t.Fatal(err)
		}
		if reply.Result["protocolVersion"] != "2025-03-26" {
			t.Fatal("protocol changed")
		}
		got, present := reply.Result["instructions"]
		if guidance == "" && present {
			t.Fatal("empty instructions leaked into legacy servers")
		}
		if guidance != "" && got != guidance {
			t.Fatal("host guidance missing")
		}
	}
}
