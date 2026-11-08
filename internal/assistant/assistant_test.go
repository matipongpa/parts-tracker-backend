package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"partlist/internal/assistant/internal/llm"
	"partlist/internal/parts"
	"partlist/internal/partstools"
)

// scriptedModel replays a fixed list of steps and records what the loop sent back.
type scriptedModel struct {
	steps    []llm.Step
	errAt    int // 1-based step index that fails; 0 = never
	err      error
	received [][]llm.ToolResult
	asked    string
	system   string
	specs    []partstools.Spec
	calls    int
}

func (m *scriptedModel) NewConversation(system string, specs []partstools.Spec) llm.Conversation {
	m.system, m.specs = system, specs
	return m
}

func (m *scriptedModel) Start(_ context.Context, q string) (llm.Step, error) {
	m.asked = q
	return m.next()
}

func (m *scriptedModel) Continue(_ context.Context, r []llm.ToolResult) (llm.Step, error) {
	m.received = append(m.received, r)
	return m.next()
}

func (m *scriptedModel) next() (llm.Step, error) {
	m.calls++
	if m.errAt == m.calls {
		return llm.Step{}, m.err
	}
	if m.calls > len(m.steps) {
		return m.steps[len(m.steps)-1], nil // keep repeating the last step
	}
	return m.steps[m.calls-1], nil
}

func fakeTool(name string, out partstools.Output, err error) partstools.Tool {
	return partstools.Tool{
		Spec: partstools.Spec{Name: name, Description: name},
		Call: func(context.Context, json.RawMessage) (partstools.Output, error) { return out, err },
	}
}

func call(id string, name string) llm.ToolCall {
	return llm.ToolCall{ID: id, Name: name, Input: json.RawMessage(`{}`)}
}

func TestAskReturnsToolRowsNotModelText(t *testing.T) {
	usage := fakeTool("get_part_usage", partstools.Output{
		Content: `{"projects":2}`,
		Parts:   []parts.Part{{ID: "1", ProjectName: "A"}, {ID: "2", ProjectName: "B"}},
	}, nil)
	search := fakeTool("search_parts", partstools.Output{
		Content: `{"count":2}`,
		Parts:   []parts.Part{{ID: "2", ProjectName: "B"}, {ID: "3", ProjectName: "C"}},
	}, nil)
	model := &scriptedModel{steps: []llm.Step{
		{Calls: []llm.ToolCall{call("u1", "get_part_usage"), call("s1", "search_parts")}},
		{Text: "  Used in A, B and C.  "},
	}}
	svc := newService([]partstools.Tool{usage, search}, model)

	got, err := svc.Ask(context.Background(), "  which projects use ABC?  ")
	if err != nil {
		t.Fatal(err)
	}
	if model.asked != "which projects use ABC?" || model.system != systemPrompt || len(model.specs) != 2 {
		t.Errorf("model got question %q, %d specs", model.asked, len(model.specs))
	}
	if got.Answer != "Used in A, B and C." {
		t.Errorf("answer = %q", got.Answer)
	}
	ids := []string{}
	for _, p := range got.Parts {
		ids = append(ids, p.ID)
	}
	if strings.Join(ids, ",") != "1,2,3" {
		t.Errorf("part ids = %v, want 1,2,3 (deduplicated across tools)", ids)
	}
	if strings.Join(got.ToolsUsed, ",") != "get_part_usage,search_parts" || got.PartsTruncated {
		t.Errorf("tools = %v truncated = %v", got.ToolsUsed, got.PartsTruncated)
	}
	if len(model.received) != 1 || len(model.received[0]) != 2 ||
		model.received[0][0].CallID != "u1" || model.received[0][0].Content != `{"projects":2}` ||
		model.received[0][1].CallID != "s1" {
		t.Errorf("results sent back = %+v (want both results, in call order, in one message)", model.received)
	}
}

func TestAskWithoutToolCallsReturnsEmptyNonNilLists(t *testing.T) {
	svc := newService(nil, &scriptedModel{steps: []llm.Step{{Text: "Hello"}}})
	got, err := svc.Ask(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if got.Parts == nil || got.ToolsUsed == nil {
		t.Errorf("lists must encode as [] not null: %+v", got)
	}
}

func TestAskRejectsBadQuestions(t *testing.T) {
	svc := newService(nil, &scriptedModel{steps: []llm.Step{{Text: "x"}}})
	for _, q := range []string{"", "   ", strings.Repeat("é", maxQuestionRunes+1)} {
		if _, err := svc.Ask(context.Background(), q); !errors.Is(err, ErrInvalidQuestion) {
			t.Errorf("Ask(%d runes) err = %v, want ErrInvalidQuestion", len([]rune(q)), err)
		}
	}
	if _, err := svc.Ask(context.Background(), strings.Repeat("é", maxQuestionRunes)); err != nil {
		t.Errorf("a question of exactly %d runes must be accepted: %v", maxQuestionRunes, err)
	}
}

func TestToolFailuresBecomeErrorResultsForTheModel(t *testing.T) {
	tools := []partstools.Tool{
		fakeTool("bad_args", partstools.Output{}, fmt.Errorf("%w: part_number must not be empty", partstools.ErrInvalidArguments)),
		fakeTool("db_down", partstools.Output{}, errors.New("pq: password=hunter2 refused")),
	}
	model := &scriptedModel{steps: []llm.Step{
		{Calls: []llm.ToolCall{call("1", "bad_args"), call("2", "db_down"), call("3", "nope")}},
		{Text: "sorry"},
	}}
	if _, err := newService(tools, model).Ask(context.Background(), "q"); err != nil {
		t.Fatal(err)
	}
	r := model.received[0]
	if !r[0].IsError || !strings.Contains(r[0].Content, "part_number must not be empty") {
		t.Errorf("invalid arguments result = %+v", r[0])
	}
	if !r[1].IsError || strings.Contains(r[1].Content, "hunter2") {
		t.Errorf("internal error must not leak details to the model: %+v", r[1])
	}
	if !r[2].IsError || !strings.Contains(r[2].Content, "unknown tool") {
		t.Errorf("unknown tool result = %+v", r[2])
	}
}

func TestFailedToolsDoNotCountAsUsed(t *testing.T) {
	tools := []partstools.Tool{fakeTool("db_down", partstools.Output{}, errors.New("boom"))}
	model := &scriptedModel{steps: []llm.Step{{Calls: []llm.ToolCall{call("1", "db_down")}}, {Text: "ok"}}}
	got, err := newService(tools, model).Ask(context.Background(), "q")
	if err != nil || len(got.ToolsUsed) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestAskStopsAfterTooManyToolRounds(t *testing.T) {
	tools := []partstools.Tool{fakeTool("search_parts", partstools.Output{Content: "{}"}, nil)}
	model := &scriptedModel{steps: []llm.Step{{Calls: []llm.ToolCall{call("1", "search_parts")}}}}
	_, err := newService(tools, model).Ask(context.Background(), "q")
	if !errors.Is(err, ErrTooManyRounds) {
		t.Fatalf("err = %v, want ErrTooManyRounds", err)
	}
	if model.calls != maxToolRounds+1 {
		t.Errorf("model called %d times, want %d", model.calls, maxToolRounds+1)
	}
}

func TestAskEmptyAnswerIsAnError(t *testing.T) {
	svc := newService(nil, &scriptedModel{steps: []llm.Step{{Text: "  \n"}}})
	if _, err := svc.Ask(context.Background(), "q"); !errors.Is(err, ErrNoAnswer) {
		t.Errorf("err = %v, want ErrNoAnswer", err)
	}
}

func TestAskPropagatesModelErrors(t *testing.T) {
	boom := errors.New("upstream down")
	tools := []partstools.Tool{fakeTool("search_parts", partstools.Output{Content: "{}"}, nil)}
	for _, errAt := range []int{1, 2} { // failing on the first request and on a follow-up
		model := &scriptedModel{
			steps: []llm.Step{{Calls: []llm.ToolCall{call("1", "search_parts")}}, {Text: "x"}},
			errAt: errAt, err: boom,
		}
		if _, err := newService(tools, model).Ask(context.Background(), "q"); !errors.Is(err, boom) {
			t.Errorf("errAt=%d: err = %v, want the model error", errAt, err)
		}
	}
}

func TestReturnedPartsAreCapped(t *testing.T) {
	many := make([]parts.Part, maxReturnedParts+50)
	for i := range many {
		many[i] = parts.Part{ID: fmt.Sprint(i + 1)}
	}
	tools := []partstools.Tool{fakeTool("search_parts", partstools.Output{Content: "{}", Parts: many}, nil)}
	model := &scriptedModel{steps: []llm.Step{{Calls: []llm.ToolCall{call("1", "search_parts")}}, {Text: "many"}}}
	got, err := newService(tools, model).Ask(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Parts) != maxReturnedParts || !got.PartsTruncated {
		t.Errorf("parts = %d truncated = %v", len(got.Parts), got.PartsTruncated)
	}
}

func TestNewValidatesProviderSettings(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"no key", Config{}, true},
		{"anthropic default provider", Config{APIKey: "k"}, false},
		{"anthropic explicit", Config{Provider: ProviderAnthropic, APIKey: "k", Model: "claude-sonnet-5-5"}, false},
		{"openai-compatible complete", Config{Provider: ProviderOpenAICompatible, APIKey: "k", BaseURL: "https://h/api/v1", Model: "m"}, false},
		{"openai-compatible without base url", Config{Provider: ProviderOpenAICompatible, APIKey: "k", Model: "m"}, true},
		{"openai-compatible without model", Config{Provider: ProviderOpenAICompatible, APIKey: "k", BaseURL: "https://h"}, true},
		{"unknown provider", Config{Provider: "gemini", APIKey: "k"}, true},
	}
	for _, tc := range cases {
		if _, err := New(nil, tc.cfg); (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr = %v", tc.name, err, tc.wantErr)
		}
	}
}
