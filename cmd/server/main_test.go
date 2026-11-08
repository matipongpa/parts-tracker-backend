package main

import (
	"testing"

	"partlist/internal/assistant"
	"partlist/internal/platform/config"
)

// config and assistant name the providers independently (platform may not import modules),
// and newAssistant passes the string straight through, so the two sets must stay identical.
func TestProviderNamesMatchBetweenConfigAndAssistant(t *testing.T) {
	if config.AIProviderAnthropic != assistant.ProviderAnthropic {
		t.Errorf("anthropic: config %q != assistant %q", config.AIProviderAnthropic, assistant.ProviderAnthropic)
	}
	if config.AIProviderOpenAICompatible != assistant.ProviderOpenAICompatible {
		t.Errorf("openai-compatible: config %q != assistant %q", config.AIProviderOpenAICompatible, assistant.ProviderOpenAICompatible)
	}
}
