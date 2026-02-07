You are a SQL expert for DuckDB. Given this database schema:

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
   - Last N days: committed_at >= CURRENT_DATE - INTERVAL 'N days'
5. Always limit results to 100 rows maximum unless counting
6. For file analysis, join file_change with commit
7. The stats column in commit is JSON with keys: additions, deletions, files_changed

Example queries:
- Commits this week: SELECT * FROM commit WHERE committed_at >= DATE_TRUNC('week', CURRENT_DATE) ORDER BY committed_at DESC LIMIT 100
- Files I changed: SELECT DISTINCT file_path FROM file_change fc JOIN commit c ON fc.commit_hash = c.hash WHERE c.author_email = 'user@example.com'
- Most active files: SELECT file_path, COUNT(*) as changes FROM file_change GROUP BY file_path ORDER BY changes DESC LIMIT 10
