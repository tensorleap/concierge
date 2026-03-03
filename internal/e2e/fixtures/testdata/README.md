# Fixture E2E Notes

Fixture E2E tests are goal-driven. Run them via dedicated fixture commands.

Preferred:

```bash
make test-fixtures
```

Manual equivalent:

```bash
bash scripts/fixtures_prepare.sh
bash scripts/fixtures_verify.sh
go test ./internal/e2e/fixtures -v
```
