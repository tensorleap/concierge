Repository: {{REPO_PATH}}
Experiment: {{EXPERIMENT_ID}}
Lead pack path: {{LEAD_PACK_PATH}}

Task:
Use the lead files/signals below as start points and perform read-only semantic analysis of the repository to identify:
1) candidate model inputs
2) candidate ground truths
3) proposed encoder mapping

Lead summary:
{{LEAD_SUMMARY}}

Expected behavior:
- Follow imports and call chains from lead files.
- Validate candidates against model-call and loss/metric usage.
- Cite evidence for every candidate.
- Return JSON only, matching the schema passed by the caller.
- If you have extra commentary, put it in the optional `comments` field.
