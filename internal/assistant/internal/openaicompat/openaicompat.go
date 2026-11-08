// Package openaicompat implements llm.Model for any server that speaks the OpenAI chat
// completions API with tool calling (OpenAI, Ollama, OpenRouter, gateways such as tokenme).
//
// It uses plain net/http: the surface needed is one endpoint, and a hand-rolled client avoids
// pinning a vendor SDK to the quirks of whichever gateway sits behind the base URL.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"partlist/internal/assistant/internal/llm"
	"partlist/internal/partstools"
)

const (
	chatPath         string        = "/chat/completions"
	maxOutputTokens  int           = 8000
	maxResponseBytes int64         = 4 << 20
	maxErrorSnippet  int           = 300
	requestTimeout   time.Duration = 120 * time.Second
)

// Model talks to an OpenAI-compatible server.
type Model struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// New returns a Model that posts to baseURL+"/chat/completions" with the given bearer key.
// baseURL is the API root as shown in the provider's docs, e.g. https://host/api/v1.
func New(baseURL string, apiKey string, model string) *Model {
	return &Model{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: requestTimeout},
	}
}

// NewConversation starts a conversation exposing the given tools.
func (m *Model) NewConversation(system string, specs []partstools.Spec) llm.Conversation {
	return &conversation{model: m, tools: toToolDefs(specs), messages: []json.RawMessage{mustMarshal(textMessage{Role: "system", Content: system})}}
}

type conversation struct {
	model *Model
	tools []toolDef
	// messages is append-only. Assistant turns are stored exactly as the server sent them, so
	// provider-specific fields (such as DeepSeek's reasoning_content) survive the tool round-trip.
	messages []json.RawMessage
}

type textMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type toolDef struct {
	Type     string      `json:"type"`
	Function toolFuncDef `json:"function"`
}

type toolFuncDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatRequest struct {
	Model     string            `json:"model"`
	Messages  []json.RawMessage `json:"messages"`
	Tools     []toolDef         `json:"tools,omitempty"`
	MaxTokens int               `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message      json.RawMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type assistantMessage struct {
	Content   *string `json:"content"`
	ToolCalls []struct {
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

func (c *conversation) Start(ctx context.Context, question string) (llm.Step, error) {
	c.messages = append(c.messages, mustMarshal(textMessage{Role: "user", Content: question}))
	return c.step(ctx)
}

func (c *conversation) Continue(ctx context.Context, results []llm.ToolResult) (llm.Step, error) {
	for _, r := range results {
		content := r.Content
		if r.IsError {
			// The chat API has no is_error flag, so mark failures in the text the model reads.
			content = "ERROR: " + content
		}
		c.messages = append(c.messages, mustMarshal(textMessage{Role: "tool", ToolCallID: r.CallID, Content: content}))
	}
	return c.step(ctx)
}

func (c *conversation) step(ctx context.Context) (llm.Step, error) {
	resp, err := c.model.post(ctx, chatRequest{Model: c.model.model, Messages: c.messages, Tools: c.tools, MaxTokens: maxOutputTokens})
	if err != nil {
		return llm.Step{}, err
	}
	if len(resp.Choices) == 0 || len(resp.Choices[0].Message) == 0 {
		return llm.Step{}, errors.New("openai-compatible response has no choices")
	}
	choice := resp.Choices[0]
	c.messages = append(c.messages, choice.Message)

	var msg assistantMessage
	if err := json.Unmarshal(choice.Message, &msg); err != nil {
		return llm.Step{}, fmt.Errorf("decode assistant message: %w", err)
	}
	switch choice.FinishReason {
	case "length":
		return llm.Step{}, llm.ErrTruncated
	case "content_filter":
		return llm.Step{}, llm.ErrRefused
	}

	step := llm.Step{}
	if msg.Content != nil {
		step.Text = *msg.Content
	}
	// Some gateways report finish_reason "stop" even when they return tool calls, so the
	// presence of tool_calls decides, not the reason.
	for _, call := range msg.ToolCalls {
		args := strings.TrimSpace(call.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		step.Calls = append(step.Calls, llm.ToolCall{ID: call.ID, Name: call.Function.Name, Input: json.RawMessage(args)})
	}
	return step, nil
}

func (m *Model) post(ctx context.Context, body chatRequest) (chatResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return chatResponse{}, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+chatPath, bytes.NewReader(payload))
	if err != nil {
		return chatResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := m.http.Do(req)
	if err != nil {
		return chatResponse{}, fmt.Errorf("openai-compatible request: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return chatResponse{}, fmt.Errorf("read response: %w", err)
	}

	var decoded chatResponse
	jsonErr := json.Unmarshal(raw, &decoded)
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return chatResponse{}, fmt.Errorf("openai-compatible request failed: status %d: %s", res.StatusCode, errorText(decoded, raw))
	}
	if jsonErr != nil {
		return chatResponse{}, fmt.Errorf("decode response: %w", jsonErr)
	}
	if decoded.Error != nil {
		return chatResponse{}, fmt.Errorf("openai-compatible request failed: %s", decoded.Error.Message)
	}
	return decoded, nil
}

func errorText(decoded chatResponse, raw []byte) string {
	if decoded.Error != nil && decoded.Error.Message != "" {
		return decoded.Error.Message
	}
	text := strings.TrimSpace(string(raw))
	if len(text) > maxErrorSnippet {
		text = text[:maxErrorSnippet]
	}
	return text
}

func toToolDefs(specs []partstools.Spec) []toolDef {
	defs := make([]toolDef, len(specs))
	for i, spec := range specs {
		defs[i] = toolDef{Type: "function", Function: toolFuncDef{Name: spec.Name, Description: spec.Description, Parameters: spec.Schema()}}
	}
	return defs
}

func mustMarshal(v any) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		// Only plain strings are marshaled here, so this cannot happen.
		panic(fmt.Sprintf("openaicompat: marshal %T: %v", v, err))
	}
	return out
}
