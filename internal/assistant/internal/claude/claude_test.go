package claude

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"partlist/internal/assistant/internal/llm"
	"partlist/internal/partstools"
)

// fakeAPI serves canned /v1/messages responses and records the request bodies it saw.
type fakeAPI struct {
	responses []string
	status    int
	requests  []map[string]any
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.requests = append(f.requests, body)

	idx := len(f.requests) - 1
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	w.Header().Set("Content-Type", "application/json")
	if f.status != 0 {
		w.WriteHeader(f.status)
	}
	_, _ = io.WriteString(w, f.responses[idx])
}

func message(stop string, content string) string {
	return `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","stop_reason":"` + stop +
		`","stop_sequence":null,"content":` + content + `,"usage":{"input_tokens":10,"output_tokens":5}}`
}

func newTestModel(t *testing.T, api *fakeAPI) *Model {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return &Model{
		client: anthropic.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0)),
		model:  "claude-opus-5-5",
	}
}

func usageSpec() partstools.Spec {
	return partstools.Spec{
		Name:        "get_part_usage",
		Description: "where used",
		Properties:  map[string]any{"part_number": map[string]any{"type": "string"}},
		Required:    []string{"part_number"},
	}
}

func TestToolRoundTrip(t *testing.T) {
	api := &fakeAPI{responses: []string{
		message("tool_use", `[{"type":"text","text":"Checking."},{"type":"tool_use","id":"toolu_1","name":"get_part_usage","input":{"part_number":"ABC"}}]`),
		message("end_turn", `[{"type":"text","text":"Used in A."}]`),
	}}
	conv := newTestModel(t, api).NewConversation("sys prompt", []partstools.Spec{usageSpec()})

	step, err := conv.Start(context.Background(), "who uses ABC?")
	if err != nil {
		t.Fatal(err)
	}
	if len(step.Calls) != 1 || step.Calls[0].ID != "toolu_1" || step.Calls[0].Name != "get_part_usage" ||
		!strings.Contains(string(step.Calls[0].Input), `"ABC"`) {
		t.Fatalf("step = %+v", step)
	}

	first := api.requests[0]
	if first["model"] != "claude-opus-5-5" {
		t.Errorf("model = %v", first["model"])
	}
	if _, forced := first["tool_choice"]; forced {
		t.Errorf("tool_choice must not be sent: current models reject forced tool use")
	}
	if _, set := first["thinking"]; set {
		t.Errorf("thinking must be left at the model default")
	}
	system, _ := first["system"].([]any)
	if len(system) != 1 || system[0].(map[string]any)["text"] != "sys prompt" {
		t.Errorf("system = %v", first["system"])
	}
	tool := first["tools"].([]any)[0].(map[string]any)
	schema := tool["input_schema"].(map[string]any)
	if tool["name"] != "get_part_usage" || schema["type"] != "object" || schema["additionalProperties"] != false ||
		schema["required"].([]any)[0] != "part_number" {
		t.Errorf("tool = %v", tool)
	}

	step, err = conv.Continue(context.Background(), []llm.ToolResult{{CallID: "toolu_1", Content: `{"projects":1}`, IsError: true}})
	if err != nil {
		t.Fatal(err)
	}
	if step.Text != "Used in A." || len(step.Calls) != 0 {
		t.Errorf("final step = %+v", step)
	}

	msgs := api.requests[1]["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("second request has %d messages, want user/assistant/user", len(msgs))
	}
	if msgs[1].(map[string]any)["role"] != "assistant" || msgs[2].(map[string]any)["role"] != "user" {
		t.Errorf("roles = %v", msgs)
	}
	result := msgs[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if result["type"] != "tool_result" || result["tool_use_id"] != "toolu_1" || result["is_error"] != true {
		t.Errorf("tool result block = %v", result)
	}
}

func TestParallelToolCallsAnswerInOneUserMessage(t *testing.T) {
	api := &fakeAPI{responses: []string{
		message("tool_use", `[{"type":"tool_use","id":"a","name":"get_part_usage","input":{}},{"type":"tool_use","id":"b","name":"get_part_usage","input":{}}]`),
		message("end_turn", `[{"type":"text","text":"ok"}]`),
	}}
	conv := newTestModel(t, api).NewConversation("s", []partstools.Spec{usageSpec()})
	step, err := conv.Start(context.Background(), "q")
	if err != nil || len(step.Calls) != 2 {
		t.Fatalf("step = %+v, %v", step, err)
	}
	if _, err := conv.Continue(context.Background(), []llm.ToolResult{{CallID: "a", Content: "1"}, {CallID: "b", Content: "2"}}); err != nil {
		t.Fatal(err)
	}
	msgs := api.requests[1]["messages"].([]any)
	if blocks := msgs[len(msgs)-1].(map[string]any)["content"].([]any); len(blocks) != 2 {
		t.Errorf("last message has %d blocks, want both tool results together", len(blocks))
	}
}

func TestStopReasons(t *testing.T) {
	cases := []struct {
		name string
		resp string
		want error
	}{
		{"refusal", message("refusal", `[]`), llm.ErrRefused},
		{"max tokens", message("max_tokens", `[{"type":"text","text":"cut"}]`), llm.ErrTruncated},
	}
	for _, tc := range cases {
		conv := newTestModel(t, &fakeAPI{responses: []string{tc.resp}}).NewConversation("s", nil)
		if _, err := conv.Start(context.Background(), "q"); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestAPIErrorsAreReturned(t *testing.T) {
	api := &fakeAPI{status: http.StatusInternalServerError, responses: []string{`{"type":"error","error":{"type":"api_error","message":"boom"}}`}}
	conv := newTestModel(t, api).NewConversation("s", nil)
	if _, err := conv.Start(context.Background(), "q"); err == nil || errors.Is(err, llm.ErrRefused) {
		t.Errorf("err = %v, want an upstream error", err)
	}
}
