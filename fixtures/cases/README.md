# Fixture Cases

Guide-native mutation cases are declared in `fixtures/cases/manifest.json` and materialized under `.fixtures/cases/`.

Each case:

- starts from a prepared `post` fixture variant, including any deterministic local runtime cleanup applied during fixture prep
- applies one reviewed patch from `fixtures/cases/patches/`
- is committed locally with fixed author/date metadata
- ships with a generated `.fixture_reset.sh` that restores the exact case commit

The current corpus is capability-first:

- one deterministic case per failure family
- one ordered composite recovery case
- all cases prefer the smallest suitable fixture source to keep them fast and reproducible

Generate and verify the cases with:

```bash
bash scripts/fixtures_prepare.sh
bash scripts/fixtures_mutate_cases.sh
bash scripts/fixtures_verify.sh
```

Use `bash scripts/fixtures_bootstrap_poetry.sh --help` if you need explicit local Poetry environments for the generated fixtures.
That bootstrap flow enforces `code-loader >= 1.0.165` in the installed Poetry env.
