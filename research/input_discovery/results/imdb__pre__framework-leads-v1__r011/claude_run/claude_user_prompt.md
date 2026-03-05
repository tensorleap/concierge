Repository: /Users/assaf/Dropbox/tensorleap/worktrees/concierge/research/inputs/.fixtures/imdb/pre
Experiment: imdb__pre__framework-leads-v1__r011
Lead pack path: /Users/assaf/Dropbox/tensorleap/worktrees/concierge/research/inputs/research/input_discovery/results/imdb__pre__framework-leads-v1__r011/lead_pack.json

Task:
Use the lead files/signals below as start points and perform read-only semantic analysis of the repository to identify:
1) candidate model inputs
2) candidate ground truths
3) proposed encoder mapping

Lead summary:
Method: framework-leads-v1
Python files scanned: 5
Files with hits: 2
Total signal hits: 3

Top lead files:
1. imdb/utils.py (score=11.0)
   - tensorflow_import: count=1, contribution=6.0
     line 3: from tensorflow.keras.preprocessing.sequence import pad_sequences
   - keras_import: count=1, contribution=5.0
     line 3: from tensorflow.keras.preprocessing.sequence import pad_sequences

2. imdb/data/preprocess.py (score=5.0)
   - keras_import: count=1, contribution=5.0
     line 2: from keras.preprocessing.text import tokenizer_from_json

Expected behavior:
- Start by validating framework direction from repository evidence.
- Follow imports and call chains from lead files.
- Validate candidates against model-call and loss/metric usage.
- Cite evidence for every candidate.
- Return JSON only, matching the schema passed by the caller.
- If you have extra commentary, put it in the optional `comments` field.
