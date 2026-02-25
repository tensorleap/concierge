# Fixture Corpus

Step 5G fixtures are materialized locally from pinned Tensorleap Hub commits.

## Source of truth

- Manifest: `fixtures/manifest.json`
- Output directory: `.fixtures/` (repo-local, gitignored)

Each fixture creates:

- `.fixtures/<id>/post`: checkout at the pinned `post_ref`
- `.fixtures/<id>/pre`: a derived local commit that strips `strip_for_pre` files

`pre` is committed locally so both `post` and `pre` remain clean git trees.

## Usage

```bash
bash scripts/fixtures_prepare.sh
bash scripts/fixtures_verify.sh
```

Both scripts are fail-fast and stop on the first fixture error.
