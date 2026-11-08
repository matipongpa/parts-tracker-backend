// Package llm is the assistant's view of a language model: a conversation that can ask for
// tool calls and be given their results. It hides provider message formats, so the loop in
// package assistant can be tested with a fake and the Claude adapter can own its own history.
package llm

import (
	"context"
	"encoding/json"
	"errors"

	"partlist/internal/partstools"
)

var (
	// ErrRefused means the model declined to answer (a safety stop), not that the call failed.
	ErrRefused error = errors.New("llm: model refused the request")
	// ErrTruncated means the answer was cut off by the output token limit.
	ErrTruncated error = errors.New("llm: answer was truncated")
)

// ToolCall is a request from the model to run one tool.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult answers one ToolCall. IsError tells the model the call failed so it can adjust.
type ToolResult struct {
	CallID  string
	Content string
	IsError bool
}

// Step is one model turn: free text, plus any tool calls it wants run before it continues.
// A Step without Calls is the final answer.
type Step struct {
	Text  string
	Calls []ToolCall
}

// Conversation is one question and its tool round-trips. It is not safe for concurrent use.
type Conversation interface {
	Start(ctx context.Context, question string) (Step, error)
	Continue(ctx context.Context, results []ToolResult) (Step, error)
}

// Model creates conversations that are allowed to call the given tools.
type Model interface {
	NewConversation(system string, tools []partstools.Spec) Conversation
}
