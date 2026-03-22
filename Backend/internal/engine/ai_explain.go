package engine

// AI-assisted explanation layer (PRD §6).
//
// Design contract (from PRD):
//   - AI explains patterns; it NEVER detects or decides them.
//   - Rule-based detection fires first (<1ms). AI runs after, as an explanation
//     enhancement layer.
//   - If the API is unavailable or slow, ExplainPatternWithAI falls back to the
//     deterministic rule-based explanation — detection is never blocked.
//   - Prompt guardrails: neutral language, no cause speculation, max 2 sentences.
//
// Configuration: set ANTHROPIC_API_KEY environment variable to enable.
// Leave it unset to run in fully rule-based mode (default).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"behaviourlens/internal/models"
)

const (
	anthropicAPIURL  = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	aiModel          = "claude-haiku-4-5-20251001"
	aiTimeout        = 3 * time.Second // keep the pipeline fast
	aiMaxTokens      = 120
)

// systemPrompt enforces the guardrails described in PRD §6.
const systemPrompt = `You are a product analytics assistant. Explain user behavior patterns in plain English for a product manager. Be precise, neutral, and brief. Never speculate beyond what the data shows. Avoid emotional language. Do not suggest causes. Maximum 2 sentences.`

// AIExplainer calls the Anthropic Claude API to generate richer pattern explanations.
// It is optional — the system is fully functional without it.
type AIExplainer struct {
	apiKey string
	client *http.Client
}

// NewAIExplainer creates an AIExplainer. Returns nil if apiKey is empty,
// signalling that AI explanations are disabled.
func NewAIExplainer(apiKey string) *AIExplainer {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	return &AIExplainer{
		apiKey: apiKey,
		client: &http.Client{Timeout: aiTimeout},
	}
}

// Explain calls Claude to produce a 1-2 sentence explanation.
// Returns an error if the API call fails; callers must fall back to rule-based.
func (a *AIExplainer) Explain(p models.Pattern, state models.UserState) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":      aiModel,
		"max_tokens": aiMaxTokens,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": buildAIPrompt(p, state)},
		},
	})
	if err != nil {
		return "", fmt.Errorf("ai_explain: marshal: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai_explain: new request: %w", err)
	}
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai_explain: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("ai_explain: api status %d: %s", resp.StatusCode, snippet)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ai_explain: decode: %w", err)
	}
	if len(result.Content) == 0 || strings.TrimSpace(result.Content[0].Text) == "" {
		return "", fmt.Errorf("ai_explain: empty response")
	}

	return strings.TrimSpace(result.Content[0].Text), nil
}

// buildAIPrompt follows the template from PRD §6.
func buildAIPrompt(p models.Pattern, state models.UserState) string {
	// Include up to the last 8 events to keep the prompt concise.
	const maxEvents = 8
	events := state.Events
	if len(events) > maxEvents {
		events = events[len(events)-maxEvents:]
	}

	parts := make([]string, 0, len(events))
	for _, e := range events {
		s := fmt.Sprintf("[%s: %s]", e.Action, e.Page)
		if dur, ok := e.Metadata["duration_ms"]; ok {
			s = fmt.Sprintf("[%s: %s, %sms]", e.Action, e.Page, dur)
		}
		parts = append(parts, s)
	}

	return fmt.Sprintf(
		"A user has performed the following actions: %s.\nDetected pattern: %s (severity: %s) on page %s.\nGenerate a 1-2 sentence explanation for a product manager.",
		strings.Join(parts, ", "), p.Type, p.Severity, p.Page,
	)
}

// ExplainPatternWithAI attempts an AI-generated explanation and falls back to the
// deterministic rule-based explanation if AI is disabled or the call fails.
// This is the primary entry point used by the consumer goroutine.
func ExplainPatternWithAI(p models.Pattern, state models.UserState, ai *AIExplainer) string {
	if ai != nil {
		text, err := ai.Explain(p, state)
		if err == nil && text != "" {
			return text
		}
		log.Printf("ai_explain: fallback for %s: %v", p.PatternID, err)
	}
	return ExplainPattern(p, state)
}
