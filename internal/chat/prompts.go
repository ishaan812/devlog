package chat

import "fmt"

const SQLGenerationPrompt = `You are a SQL expert for DuckDB. Given this database schema:

%s

Write a SQL query to answer the following question: %s

Important rules:
1. Return ONLY the SQL query, no explanation or markdown formatting
2. Use DuckDB syntax (similar to PostgreSQL)
3. For date filtering, use committed_at column with TIMESTAMP comparisons
4. Common date patterns:
   - Today: committed_at >= CURRENT_DATE
   - This week: committed_at >= DATE_TRUNC('week', CURRENT_DATE)
   - This month: committed_at >= DATE_TRUNC('month', CURRENT_DATE)
   - Last N days: committed_at >= CURRENT_DATE - INTERVAL '%d days'
5. Always limit results to 100 rows maximum unless counting
6. For file analysis, join file_change with commit
7. The stats column in commit is JSON with keys: additions, deletions, files_changed

Example queries:
- Commits this week: SELECT * FROM commit WHERE committed_at >= DATE_TRUNC('week', CURRENT_DATE) ORDER BY committed_at DESC LIMIT 100
- Files I changed: SELECT DISTINCT file_path FROM file_change fc JOIN commit c ON fc.commit_hash = c.hash WHERE c.author_email = 'user@example.com'
- Most active files: SELECT file_path, COUNT(*) as changes FROM file_change GROUP BY file_path ORDER BY changes DESC LIMIT 10`

const SummarizationPrompt = `You are a helpful assistant summarizing development activity.
Given the following query results about git commits and file changes, provide a clear and concise summary.

User's original question: %s

Query results (JSON):
%s

Instructions:
1. Summarize the key findings in a natural, conversational way
2. Highlight important patterns or notable commits
3. If showing commits, mention the commit messages and dates
4. If showing files, group them logically if possible
5. Be concise but informative
6. If no results, say so clearly and suggest why that might be`

const TimeFilterPrompt = `Extract any time-related filters from this question and return them in a structured format.

Question: %s

Return a JSON object with:
- "type": one of "today", "week", "month", "days", "range", "none"
- "days": number of days (only if type is "days")
- "start": start date in YYYY-MM-DD format (only if type is "range")
- "end": end date in YYYY-MM-DD format (only if type is "range")

Examples:
- "what did I do today" -> {"type": "today"}
- "this week's commits" -> {"type": "week"}
- "last 30 days" -> {"type": "days", "days": 30}
- "commits in January" -> {"type": "range", "start": "2024-01-01", "end": "2024-01-31"}
- "all my commits" -> {"type": "none"}

Return ONLY the JSON object, no explanation.`

func BuildSQLPrompt(schema, question string) string {
	return fmt.Sprintf(SQLGenerationPrompt, schema, question)
}

func BuildSummarizationPrompt(question, results string) string {
	return fmt.Sprintf(SummarizationPrompt, question, results)
}

func BuildTimeFilterPrompt(question string) string {
	return fmt.Sprintf(TimeFilterPrompt, question)
}
