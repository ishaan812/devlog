package llm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type BedrockClient struct {
	accessKeyID     string
	secretAccessKey string
	region          string
	model           string
	client          *http.Client
}

func NewBedrockClient(accessKeyID, secretAccessKey, region, model string) *BedrockClient {
	if region == "" {
		region = "us-east-1"
	}
	if model == "" {
		model = "anthropic.claude-sonnet-4-5-20250929-v1:0"
	}
	return &BedrockClient{
		accessKeyID:     accessKeyID,
		secretAccessKey: secretAccessKey,
		region:          region,
		model:           model,
		client:          &http.Client{},
	}
}

type bedrockClaudeRequest struct {
	AnthropicVersion string            `json:"anthropic_version"`
	MaxTokens        int               `json:"max_tokens"`
	Messages         []bedrockMessage  `json:"messages"`
	System           string            `json:"system,omitempty"`
}

type bedrockMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type bedrockClaudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (c *BedrockClient) Complete(ctx context.Context, prompt string) (string, error) {
	messages := []Message{
		{Role: "user", Content: prompt},
	}
	return c.ChatComplete(ctx, messages)
}

func (c *BedrockClient) ChatComplete(ctx context.Context, messages []Message) (string, error) {
	var systemPrompt string
	var bedrockMessages []bedrockMessage

	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		bedrockMessages = append(bedrockMessages, bedrockMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	reqBody := bedrockClaudeRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        4096,
		Messages:         bedrockMessages,
		System:           systemPrompt,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke",
		c.region, c.model)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Sign the request with AWS Signature Version 4
	if err := c.signRequest(req, jsonBody); err != nil {
		return "", fmt.Errorf("failed to sign request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Bedrock API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result bedrockClaudeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	var text string
	for _, content := range result.Content {
		if content.Type == "text" {
			text += content.Text
		}
	}

	return strings.TrimSpace(text), nil
}

// signRequest signs the request using AWS Signature Version 4
func (c *BedrockClient) signRequest(req *http.Request, payload []byte) error {
	now := time.Now().UTC()
	datestamp := now.Format("20060102")
	amzdate := now.Format("20060102T150405Z")

	service := "bedrock"
	host := req.URL.Host

	// Create canonical request
	canonicalURI := req.URL.Path
	canonicalQuerystring := req.URL.RawQuery

	payloadHash := sha256Hash(payload)

	req.Header.Set("host", host)
	req.Header.Set("x-amz-date", amzdate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		req.Header.Get("Content-Type"), host, payloadHash, amzdate)

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method, canonicalURI, canonicalQuerystring,
		canonicalHeaders, signedHeaders, payloadHash)

	// Create string to sign
	algorithm := "AWS4-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", datestamp, c.region, service)
	stringToSign := fmt.Sprintf("%s\n%s\n%s\n%s",
		algorithm, amzdate, credentialScope, sha256Hash([]byte(canonicalRequest)))

	// Calculate signature
	signingKey := getSignatureKey(c.secretAccessKey, datestamp, c.region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// Add authorization header
	authHeader := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, c.accessKeyID, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)

	return nil
}

func sha256Hash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func getSignatureKey(secretKey, datestamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}
