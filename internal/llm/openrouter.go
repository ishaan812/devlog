package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type OpenRouterClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenRouterClient(apiKey, model string) *OpenRouterClient {
	return &OpenRouterClient{
		baseURL: "https://openrouter.ai/api/v1",
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{},
	}
}

type openRouterChatRequest struct {
	Model    string              `json:"model"`
	Messages []openRouterMessage `json:"messages"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterChatResponse struct {
	Choices []struct {
		Message openRouterMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func (c *OpenRouterClient) Complete(ctx context.Context, prompt string) (string, error) {
	messages := []Message{
		{Role: "user", Content: prompt},
	}
	return c.ChatComplete(ctx, messages)
}

func (c *OpenRouterClient) ChatComplete(ctx context.Context, messages []Message) (string, error) {
	openRouterMessages := make([]openRouterMessage, len(messages))
	for i, m := range messages {
		openRouterMessages[i] = openRouterMessage(m)
	}

	reqBody := openRouterChatRequest{
		Model:    c.model,
		Messages: openRouterMessages,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/ishaan812/devlog") // Optional but recommended
	req.Header.Set("X-Title", "DevLog")                                   // Optional but recommended

	resp, err := c.client.Do(req)
	if err != nil {
		printCurlCommand("POST", c.baseURL+"/chat/completions", req.Header, jsonBody)
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		printCurlCommand("POST", c.baseURL+"/chat/completions", req.Header, jsonBody)
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result openRouterChatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		printCurlCommand("POST", c.baseURL+"/chat/completions", req.Header, jsonBody)
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if result.Error != nil {
		printCurlCommand("POST", c.baseURL+"/chat/completions", req.Header, jsonBody)
		return "", fmt.Errorf("OpenRouter API error: %s (code %d)", result.Error.Message, result.Error.Code)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// printCurlCommand prints the equivalent curl command for debugging
func printCurlCommand(method, url string, headers http.Header, body []byte) {
	fmt.Fprintf(os.Stderr, "\n[DEBUG] Equivalent curl command:\n")
	fmt.Fprintf(os.Stderr, "curl --location '%s' \\\n", url)

	for key, values := range headers {
		for _, value := range values {
			fmt.Fprintf(os.Stderr, "  --header '%s: %s' \\\n", key, value)
		}
	}

	if len(body) > 0 {
		// Pretty print JSON for readability
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, body, "", "  "); err == nil {
			fmt.Fprintf(os.Stderr, "  --data '%s'\n\n", prettyJSON.String())
		} else {
			fmt.Fprintf(os.Stderr, "  --data '%s'\n\n", string(body))
		}
	}
}
