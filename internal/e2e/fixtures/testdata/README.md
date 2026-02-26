# Fixture E2E Notes

Fixture E2E tests are opt-in and only run when `CONCIERGE_RUN_FIXTURE_E2E=1`.

Before running them, materialize fixtures:

```bash
bash scripts/fixtures_prepare.sh
```

Then run:

```bash
CONCIERGE_RUN_FIXTURE_E2E=1 go test ./internal/e2e/fixtures -v
```
