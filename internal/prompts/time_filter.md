Extract any time-related filters from this question and return them in a structured format.

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

Return ONLY the JSON object, no explanation.
