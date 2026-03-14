Progress has stalled.

Re-evaluate the session with stronger QA judgment:

- If the terminal is no longer moving and there is no sensible next input, stop instead of spinning.
- If a post-fixture path is now available, you may inspect it to compare expected behavior against the observed run.
- If the issue is clearly a product defect that warrants a repair loop, emit `STOP_FIX`.
- If the session is blocked with no useful recovery path, emit `STOP_DEADEND`.
