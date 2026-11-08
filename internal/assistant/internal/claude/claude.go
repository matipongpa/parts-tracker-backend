// Package claude implements llm.Model on the Claude Messages API with a manual tool loop.
// The loop is manual (rather than the SDK tool runner) so the caller can see every tool result.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"partlist/internal/assistant/internal/llm"
	"partlist/internal/partstools"
)

// maxOutputTokens bounds one non-streaming response; answers are short summaries.
const maxOutputTokens int64 = 8000

// Model talks to the Claude API.
type Model struct {
	client anthropic.Client
	model  string
}

// New returns a Model that calls the given Claude model id with the given API key.
func New(apiKey string, model string) *Model {
	return &Model{client: anthropic.NewClient(option.WithAPIKey(apiKey)), model: model}
}

// NewConversation starts a conversation exposing the given tools.
func (m *Model) NewConversation(system string, specs []partstools.Spec) llm.Conversation {
	return &conversation{client: m.client, model: m.model, system: system, tools: toToolParams(specs)}
}

type conversation struct {
	client   anthropic.Client
	model    string
	system   string
	tools    []anthropic.ToolUnionParam
	messages []anthropic.MessageParam
}

func (c *conversation) Start(ctx context.Context, question string) (llm.Step, error) {
	c.messages = append(c.messages, anthropic.NewUserMessage(anthropic.NewTextBlock(question)))
	return c.step(ctx)
}

func (c *conversation) Continue(ctx context.Context, results []llm.ToolResult) (llm.Step, error) {
	// All results of one assistant turn go back together in a single user message.
	blocks := make([]anthropic.ContentBlockParamUnion, len(results))
	for i, r := range results {
		blocks[i] = anthropic.NewToolResultBlock(r.CallID, r.Content, r.IsError)
	}
	c.messages = append(c.messages, anthropic.NewUserMessage(blocks...))
	return c.step(ctx)
}

func (c *conversation) step(ctx context.Context) (llm.Step, error) {
	// Thinking and effort are left at the model's defaults. Forced tool_choice is not used:
	// it is rejected by current models, and "auto" plus the system prompt is enough here.
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: maxOutputTokens,
		System:    []anthropic.TextBlockParam{{Text: c.system}},
		Messages:  c.messages,
		Tools:     c.tools,
	})
	if err != nil {
		return llm.Step{}, fmt.Errorf("claude request: %w", err)
	}
	// Append the whole response, thinking blocks included, so the history stays append-only.
	c.messages = append(c.messages, resp.ToParam())

	switch resp.StopReason {
	case anthropic.StopReasonRefusal:
		return llm.Step{}, llm.ErrRefused
	case anthropic.StopReasonMaxTokens:
		return llm.Step{}, llm.ErrTruncated
	}

	var text strings.Builder
	var calls []llm.ToolCall
	for _, block := range resp.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			text.WriteString(variant.Text)
		case anthropic.ToolUseBlock:
			calls = append(calls, llm.ToolCall{ID: variant.ID, Name: variant.Name, Input: json.RawMessage(variant.JSON.Input.Raw())})
		}
	}
	if resp.StopReason != anthropic.StopReasonToolUse {
		calls = nil
	}
	return llm.Step{Text: text.String(), Calls: calls}, nil
}

func toToolParams(specs []partstools.Spec) []anthropic.ToolUnionParam {
	tools := make([]anthropic.ToolUnionParam, len(specs))
	for i, spec := range specs {
		schema := spec.Schema()
		tool := anthropic.ToolParam{
			Name:        spec.Name,
			Description: anthropic.String(spec.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties:  spec.Properties,
				Required:    spec.Required,
				ExtraFields: map[string]any{"additionalProperties": schema["additionalProperties"]},
			},
		}
		tools[i] = anthropic.ToolUnionParam{OfTool: &tool}
	}
	return tools
}
