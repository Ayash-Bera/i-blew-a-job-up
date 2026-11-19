package tagging

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"firstrole/internal/models"
)

var errInvalidUsage = errors.New("invalid model token usage")

func (c *Client) requestBody(content string) ([]byte, error) {
	return c.requestBodyWithInstruction(content, Prompt)
}

func (c *Client) requestBodyWithInstruction(content, instruction string) ([]byte, error) {
	switch c.Provider {
	case "gemini":
		return json.Marshal(map[string]any{
			"systemInstruction": map[string]any{"parts": []any{map[string]string{"text": instruction}}},
			"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": content}}}},
			"generationConfig":  map[string]any{"responseMimeType": "application/json", "responseJsonSchema": Schema(), "maxOutputTokens": 2048, "thinkingConfig": map[string]string{"thinkingLevel": "MINIMAL"}},
		})
	case "", "compatible":
		return json.Marshal(map[string]any{"model": c.Model, "temperature": 0, "max_tokens": 700, "messages": []map[string]string{{"role": "system", "content": instruction}, {"role": "user", "content": content}}, "response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "job_tags", "strict": true, "schema": Schema()}}})
	default:
		return nil, errors.New("unsupported model provider")
	}
}

func (c *Client) responseText(data []byte, usage *models.Usage) (string, error) {
	if c.Provider == "gemini" {
		return geminiResponse(data, usage)
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string  `json:"content"`
				Refusal *string `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     *int `json:"prompt_tokens"`
			CompletionTokens *int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", err
	}
	if envelope.Usage != nil {
		usage.InputTokens = envelope.Usage.PromptTokens
		usage.OutputTokens = envelope.Usage.CompletionTokens
	}
	if len(envelope.Choices) != 1 || envelope.Choices[0].Message.Refusal != nil || envelope.Choices[0].FinishReason != "stop" {
		return "", errors.New("model response was incomplete or refused")
	}
	return envelope.Choices[0].Message.Content, nil
}

func geminiResponse(data []byte, usage *models.Usage) (string, error) {
	var envelope struct {
		Candidates []struct {
			Content struct {
				Parts []map[string]json.RawMessage `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
		Usage json.RawMessage `json:"usageMetadata"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", err
	}
	if len(envelope.Usage) > 0 && string(envelope.Usage) != "null" {
		if err := geminiUsage(envelope.Usage, usage); err != nil {
			return "", err
		}
	}
	if envelope.PromptFeedback.BlockReason != "" && envelope.PromptFeedback.BlockReason != "BLOCK_REASON_UNSPECIFIED" || len(envelope.Candidates) != 1 || envelope.Candidates[0].FinishReason != "STOP" {
		return "", errors.New("model response was incomplete or refused")
	}
	var output strings.Builder
	for _, part := range envelope.Candidates[0].Content.Parts {
		var thought bool
		if raw, ok := part["thought"]; ok {
			if err := json.Unmarshal(raw, &thought); err != nil {
				return "", errors.New("invalid model thought flag")
			}
		}
		if thought {
			continue
		}
		for key := range part {
			if key != "text" && key != "thought" && key != "thoughtSignature" {
				return "", errors.New("unexpected nontext model output")
			}
		}
		var text string
		raw, ok := part["text"]
		if !ok || string(raw) == "null" || json.Unmarshal(raw, &text) != nil {
			return "", errors.New("missing model output text")
		}
		output.WriteString(text)
	}
	if strings.TrimSpace(output.String()) == "" {
		return "", errors.New("empty model output")
	}
	return output.String(), nil
}

func geminiUsage(data []byte, usage *models.Usage) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return errInvalidUsage
	}
	for _, key := range []string{"promptTokenCount", "candidatesTokenCount", "cachedContentTokenCount", "thoughtsTokenCount", "toolUsePromptTokenCount", "totalTokenCount"} {
		if raw, ok := fields[key]; ok && strings.TrimSpace(string(raw)) == "null" {
			return fmt.Errorf("%w: null count", errInvalidUsage)
		}
	}
	var counts struct {
		Prompt     *int `json:"promptTokenCount"`
		Candidates *int `json:"candidatesTokenCount"`
		Cached     int  `json:"cachedContentTokenCount"`
		Thoughts   int  `json:"thoughtsTokenCount"`
		Tool       int  `json:"toolUsePromptTokenCount"`
		Total      *int `json:"totalTokenCount"`
	}
	if err := json.Unmarshal(data, &counts); err != nil {
		return fmt.Errorf("%w: malformed counts", errInvalidUsage)
	}
	usage.InputTokens = counts.Prompt
	usage.CachedInputTokens = &counts.Cached
	usage.ThinkingTokens = &counts.Thoughts
	if counts.Cached < 0 || counts.Thoughts < 0 || counts.Tool != 0 {
		return errInvalidUsage
	}
	if counts.Candidates != nil {
		output, ok := sumCounts(*counts.Candidates, counts.Thoughts)
		if !ok {
			return errInvalidUsage
		}
		usage.OutputTokens = &output
	}
	if !validUsage(*usage) || counts.Total != nil && *counts.Total < 0 {
		return errInvalidUsage
	}
	if counts.Total != nil && usage.InputTokens != nil && usage.OutputTokens != nil {
		total, ok := sumCounts(*usage.InputTokens, *usage.OutputTokens)
		if !ok || total != *counts.Total {
			return fmt.Errorf("%w: inconsistent total", errInvalidUsage)
		}
	}
	return nil
}

func sumCounts(a, b int) (int, bool) {
	if a < 0 || b < 0 || b > int(^uint(0)>>1)-a {
		return 0, false
	}
	return a + b, true
}

func validUsage(u models.Usage) bool {
	for _, n := range []*int{u.InputTokens, u.OutputTokens, u.CachedInputTokens, u.ThinkingTokens} {
		if n != nil && *n < 0 {
			return false
		}
	}
	return !(u.CachedInputTokens != nil && u.InputTokens != nil && *u.CachedInputTokens > *u.InputTokens || u.ThinkingTokens != nil && u.OutputTokens != nil && *u.ThinkingTokens > *u.OutputTokens)
}

func usageCost(u models.Usage) *float64 {
	if u.InputTokens == nil || u.OutputTokens == nil || u.InputUSDPerMillion == nil || u.OutputUSDPerMillion == nil {
		return nil
	}
	cached := 0
	if u.CachedInputTokens != nil {
		cached = *u.CachedInputTokens
	}
	cachedCost := 0.0
	if cached > 0 {
		if u.CachedInputUSDPerMillion == nil {
			return nil
		}
		cachedCost = float64(cached) * *u.CachedInputUSDPerMillion
	}
	cost := (float64(*u.InputTokens-cached)**u.InputUSDPerMillion + cachedCost + float64(*u.OutputTokens)**u.OutputUSDPerMillion) / 1e6
	if cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return nil
	}
	return &cost
}
