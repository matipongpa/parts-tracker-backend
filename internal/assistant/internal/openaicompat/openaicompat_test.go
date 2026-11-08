package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"partlist/internal/assistant/internal/llm"
	"partlist/internal/partstools"
)

type fakeAPI struct {
	responses []string
	status    int
	requests  []map[string]any
	paths     []string
	auth      []string
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.requests = append(f.requests, body)
	f.paths = append(f.paths, r.URL.Path)
	f.auth = append(f.auth, r.Header.Get("Authorization"))

	idx := min(len(f.requests)-1, len(f.responses)-1)
	w.Header().Set("Content-Type", "application/json")
	if f.status != 0 {
		w.WriteHeader(f.status)
	}
	_, _ = io.WriteString(w, f.responses[idx])
}

func reply(finish string, message string) string {
	return `{"id":"x","choices":[{"index":0,"finish_reason":"` + finish + `","message":` + message + `}]}`
}

func newModel(t *testing.T, api *fakeAPI) *Model {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return New(srv.URL+"/api/v1/", "secret-key", "deepseek-ai/deepseek-v4-flash-0731")
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
		reply("tool_calls", `{"role":"assistant","content":null,"reasoning_content":"thinking...","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_part_usage","arguments":"{\"part_number\":\"ABC\"}"}}]}`),
		reply("stop", `{"role":"assistant","content":"Used in A."}`),
	}}
	conv := newModel(t, api).NewConversation("sys prompt", []partstools.Spec{usageSpec()})

	step, err := conv.Start(context.Background(), "who uses ABC?")
	if err != nil {
		t.Fatal(err)
	}
	if len(step.Calls) != 1 || step.Calls[0].ID != "call_1" || step.Calls[0].Name != "get_part_usage" ||
		string(step.Calls[0].Input) != `{"part_number":"ABC"}` {
		t.Fatalf("step = %+v", step)
	}

	if api.paths[0] != "/api/v1/chat/completions" || api.auth[0] != "Bearer secret-key" {
		t.Errorf("path %q, auth %q", api.paths[0], api.auth[0])
	}
	first := api.requests[0]
	if first["model"] != "deepseek-ai/deepseek-v4-flash-0731" {
		t.Errorf("model = %v", first["model"])
	}
	if _, streaming := first["stream"]; streaming {
		t.Errorf("stream must not be set")
	}
	msgs := first["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[0].(map[string]any)["content"] != "sys prompt" ||
		msgs[1].(map[string]any)["role"] != "user" {
		t.Errorf("messages = %v", msgs)
	}
	tool := first["tools"].([]any)[0].(map[string]any)
	fn := tool["function"].(map[string]any)
	params := fn["parameters"].(map[string]any)
	if tool["type"] != "function" || fn["name"] != "get_part_usage" || params["type"] != "object" ||
		params["additionalProperties"] != false || params["required"].([]any)[0] != "part_number" {
		t.Errorf("tool = %v", tool)
	}

	step, err = conv.Continue(context.Background(), []llm.ToolResult{{CallID: "call_1", Content: `{"projects":1}`}})
	if err != nil {
		t.Fatal(err)
	}
	if step.Text != "Used in A." || len(step.Calls) != 0 {
		t.Errorf("final step = %+v", step)
	}

	second := api.requests[1]["messages"].([]any)
	if len(second) != 4 {
		t.Fatalf("second request has %d messages, want system/user/assistant/tool", len(second))
	}
	assistant := second[2].(map[string]any)
	if assistant["role"] != "assistant" || assistant["reasoning_content"] != "thinking..." || assistant["tool_calls"] == nil {
		t.Errorf("assistant turn must be echoed verbatim, got %v", assistant)
	}
	toolMsg := second[3].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "call_1" || toolMsg["content"] != `{"projects":1}` {
		t.Errorf("tool message = %v", toolMsg)
	}
}

func TestEveryParallelResultGetsItsOwnToolMessage(t *testing.T) {
	api := &fakeAPI{responses: []string{
		reply("stop", `{"role":"assistant","content":"","tool_calls":[{"id":"a","type":"function","function":{"name":"x","arguments":""}},{"id":"b","type":"function","function":{"name":"x","arguments":"{}"}}]}`),
		reply("stop", `{"role":"assistant","content":"ok"}`),
	}}
	conv := newModel(t, api).NewConversation("s", nil)
	step, err := conv.Start(context.Background(), "q")
	if err != nil || len(step.Calls) != 2 {
		t.Fatalf("step = %+v, %v (tool_calls must win over finish_reason stop)", step, err)
	}
	if string(step.Calls[0].Input) != "{}" {
		t.Errorf("empty arguments must become {}, got %q", step.Calls[0].Input)
	}
	_, err = conv.Continue(context.Background(), []llm.ToolResult{
		{CallID: "a", Content: "1"}, {CallID: "b", Content: "bad things", IsError: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs := api.requests[1]["messages"].([]any)
	last2 := msgs[len(msgs)-2:]
	if last2[0].(map[string]any)["tool_call_id"] != "a" || last2[1].(map[string]any)["tool_call_id"] != "b" {
		t.Errorf("tool messages = %v", last2)
	}
	if got := last2[1].(map[string]any)["content"]; got != "ERROR: bad things" {
		t.Errorf("failed result must be flagged in text, got %v", got)
	}
}

func TestNoToolsMeansNoToolsField(t *testing.T) {
	api := &fakeAPI{responses: []string{reply("stop", `{"role":"assistant","content":"hi"}`)}}
	if _, err := newModel(t, api).NewConversation("s", nil).Start(context.Background(), "q"); err != nil {
		t.Fatal(err)
	}
	if _, has := api.requests[0]["tools"]; has {
		t.Errorf("tools must be omitted when there are none")
	}
}

func TestFinishReasons(t *testing.T) {
	cases := []struct {
		name   string
		finish string
		want   error
	}{
		{"length", "length", llm.ErrTruncated},
		{"content filter", "content_filter", llm.ErrRefused},
	}
	for _, tc := range cases {
		api := &fakeAPI{responses: []string{reply(tc.finish, `{"role":"assistant","content":"x"}`)}}
		_, err := newModel(t, api).NewConversation("s", nil).Start(context.Background(), "q")
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		contains string
	}{
		{"http error with message", http.StatusUnauthorized, `{"error":{"message":"invalid api key"}}`, "invalid api key"},
		{"http error with plain body", http.StatusBadGateway, `upstream exploded`, "upstream exploded"},
		{"200 with error object", 0, `{"error":{"message":"model not found"}}`, "model not found"},
		{"no choices", 0, `{"choices":[]}`, "no choices"},
		{"not json", 0, `<html>`, "decode response"},
	}
	for _, tc := range cases {
		api := &fakeAPI{status: tc.status, responses: []string{tc.body}}
		_, err := newModel(t, api).NewConversation("s", nil).Start(context.Background(), "q")
		if err == nil || !strings.Contains(err.Error(), tc.contains) {
			t.Errorf("%s: err = %v, want it to contain %q", tc.name, err, tc.contains)
		}
		if err != nil && strings.Contains(err.Error(), "secret-key") {
			t.Errorf("%s: API key leaked into the error: %v", tc.name, err)
		}
	}
}

func TestContextCancellationIsReturned(t *testing.T) {
	api := &fakeAPI{responses: []string{reply("stop", `{"role":"assistant","content":"x"}`)}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newModel(t, api).NewConversation("s", nil).Start(ctx, "q"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
