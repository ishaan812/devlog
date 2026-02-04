package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/llm"
)

// Pipeline handles question-answering using LLM and database.
type Pipeline struct {
	client   llm.Client
	database *sql.DB
	verbose  bool
}

// NewPipeline creates a new Pipeline.
func NewPipeline(client llm.Client, database *sql.DB, verbose bool) *Pipeline {
	return &Pipeline{
		client:   client,
		database: database,
		verbose:  verbose,
	}
}

type TimeFilter struct {
	Type  string `json:"type"`
	Days  int    `json:"days,omitempty"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

func (p *Pipeline) Ask(ctx context.Context, question string) (string, error) {
	// Step 1: Generate SQL query
	if p.verbose {
		fmt.Println("[DEBUG] Generating SQL query...")
	}
	sqlQuery, err := p.GenerateSQL(ctx, question)
	if err != nil {
		return "", fmt.Errorf("failed to generate SQL: %w", err)
	}
	if p.verbose {
		fmt.Printf("[DEBUG] Generated SQL: %s\n", sqlQuery)
	}
	// Step 2: Execute query
	results, err := p.ExecuteQuery(sqlQuery)
	if err != nil {
		// Try to recover with a simpler query
		if p.verbose {
			fmt.Printf("[DEBUG] Query failed, trying fallback: %v\n", err)
		}
		results, err = p.fallbackQuery(ctx, question)
		if err != nil {
			return "", fmt.Errorf("failed to execute query: %w", err)
		}
	}
	if p.verbose {
		fmt.Printf("[DEBUG] Query returned %d results\n", len(results))
	}
	// Step 3: Summarize results
	summary, err := p.Summarize(ctx, question, results)
	if err != nil {
		return "", fmt.Errorf("failed to summarize results: %w", err)
	}

	return summary, nil
}

func (p *Pipeline) GenerateSQL(ctx context.Context, question string) (string, error) {
	schema := db.GetSchemaDescription()
	prompt := BuildSQLPrompt(schema, question)

	response, err := p.client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}

	// Clean up the response - remove markdown code blocks if present
	sql := cleanSQL(response)
	return sql, nil
}

// ExecuteQuery executes a SQL query and returns results.
func (p *Pipeline) ExecuteQuery(query string) ([]map[string]any, error) {
	normalizedQuery := strings.ToUpper(strings.TrimSpace(query))
	if !strings.HasPrefix(normalizedQuery, "SELECT") {
		return nil, fmt.Errorf("only SELECT queries are allowed")
	}
	repo := db.NewRepository(p.database)
	return repo.ExecuteQuery(context.Background(), query)
}

// Summarize generates a summary of query results.
func (p *Pipeline) Summarize(ctx context.Context, question string, results []map[string]any) (string, error) {
	resultsJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal results: %w", err)
	}

	// Truncate if too long
	resultsStr := string(resultsJSON)
	if len(resultsStr) > 10000 {
		resultsStr = resultsStr[:10000] + "\n... (truncated)"
	}

	prompt := BuildSummarizationPrompt(question, resultsStr)

	messages := []llm.Message{
		{Role: "system", Content: "You are a helpful assistant that summarizes development activity data."},
		{Role: "user", Content: prompt},
	}

	return p.client.ChatComplete(ctx, messages)
}

func (p *Pipeline) ParseTimeFilter(ctx context.Context, question string) (*TimeFilter, error) {
	prompt := BuildTimeFilterPrompt(question)

	response, err := p.client.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Clean up response
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var filter TimeFilter
	if err := json.Unmarshal([]byte(response), &filter); err != nil {
		// Default to no filter on parse error
		return &TimeFilter{Type: "none"}, nil
	}

	return &filter, nil
}

func (p *Pipeline) fallbackQuery(ctx context.Context, question string) ([]map[string]any, error) {
	query := `
		SELECT c.hash, c.message, c.author_email, c.committed_at, c.stats
		FROM commits c ORDER BY c.committed_at DESC LIMIT 20
	`
	repo := db.NewRepository(p.database)
	return repo.ExecuteQuery(ctx, query)
}

func cleanSQL(response string) string {
	// Remove markdown code blocks
	re := regexp.MustCompile("```(?:sql)?\\s*([\\s\\S]*?)```")
	matches := re.FindStringSubmatch(response)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	// Remove any leading/trailing whitespace and newlines
	sql := strings.TrimSpace(response)
	// Remove any "SQL:" or similar prefixes
	prefixes := []string{"SQL:", "Query:", "sql:", "query:"}
	for _, prefix := range prefixes {
		sql = strings.TrimPrefix(sql, prefix)
	}
	return strings.TrimSpace(sql)
}
