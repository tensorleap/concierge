Repository: /Users/assaf/Dropbox/tensorleap/worktrees/concierge/research/inputs/.fixtures/webinar/pre
Experiment: webinar__pre__pytorch-leads-v1__r001
Lead pack path: /Users/assaf/Dropbox/tensorleap/worktrees/concierge/research/inputs/research/input_discovery/results/webinar__pre__pytorch-leads-v1__r001/lead_pack.json

Task:
Use the lead files/signals below as start points and perform read-only semantic analysis of the repository to identify:
1) candidate model inputs
2) candidate ground truths
3) proposed encoder mapping

Lead summary:
Method: pytorch-leads-v1
Python files scanned: 5
Files with hits: 2
Total signal hits: 4

Top lead files:
1. webinar/utils/metrics.py (score=10.0)
   - batch_unpack_loop: count=2, contribution=10.0
     line 121: for i, prediction in enumerate(prediction_detected):
     line 140: for k, gt_detection in enumerate(gt_detected):

2. leap_integration.py (score=6.0)
   - main_entry: count=1, contribution=3.0
     line 51: if __name__ == '__main__':
   - model_call: count=1, contribution=3.0
     line 22: y_pred = model([img])

Expected behavior:
- Follow imports and call chains from lead files.
- Validate candidates against model-call and loss/metric usage.
- Cite evidence for every candidate.
- Return JSON only, matching the schema passed by the caller.
