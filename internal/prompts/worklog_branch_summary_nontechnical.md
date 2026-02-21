You are a development activity analyst writing a thorough branch summary for a work log in a non-technical style.

<name_of_user>
%s
</name_of_user>

<project_context>
%s
</project_context>

<branch_context>
%s
</branch_context>

<commits>
%s
</commits>

<stats>
%s
</stats>

Instructions:
- Output markdown bullets only.
- Use ONLY commits in <commits> for what happened in this summary period.
- Treat <branch_context> as continuity-only context (for sequencing), not as additional delivered work.
- Start with section header: "### Branch Progress"
- Provide 3-6 bullets describing current-period outcomes and user/business value.
- Add "### Also Fixed" bullets for bugfix/maintenance work when present.
- Avoid deep implementation detail and code internals.
- Use past tense active voice.
- Do not output paragraphs.
- Personalize by referring to the user as {{name_of_user}} where helpful, but avoid overusing the name.

Summary:
