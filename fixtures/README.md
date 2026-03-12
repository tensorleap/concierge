# Fixture Corpus

Step 5G fixtures are materialized locally from pinned Tensorleap Hub commits.

## Source of truth

- Manifest: `fixtures/manifest.json`
- Output directory: `.fixtures/` (repo-local, gitignored)

Each fixture creates:

- `.fixtures/<id>/post`: checkout at the pinned `post_ref`, plus a deterministic local cleanup commit when fixture prep must refresh runtime metadata such as the minimum `code-loader` pin
- `.fixtures/<id>/pre`: a derived local commit that strips `strip_for_pre` files or directories
- `.fixtures/cases/<case-id>`: a derived local commit that applies one reviewed guide-native mutation patch to a `post` fixture
- Invariant: `.fixtures/<id>/pre` must not contain root-level `leap*` files.
- Invariant: `.fixtures/<id>/pre` must not contain files with `tensorleap` in their contents, except `pyproject.toml` which is preserved as an essential project file.
- Invariant: `.fixtures/<id>/pre` must not contain Python files importing `code_loader`, `inner_leap_binder`, or `leapbinder_decorators`.
- Invariant: `.fixtures/<id>/pre` must not contain Tensorleap-only paths such as `tensorleap_folder/`, `.tensorleap/`, or `leap_mapping*.yaml`.
- Invariant: `.fixtures/<id>/post` must keep `leap.yaml` pointed at `leap_integration.py`.
- Invariant: `.fixtures/<id>/post` must pin `code-loader >= 1.0.165` so local integration-script execution yields validator output, even if fixture prep has to apply that pin locally after cloning the upstream `post_ref`.
- Invariant: generated cases must stay clean git trees and ship a working `.fixture_reset.sh`.

`pre` is committed locally so both `post` and `pre` remain clean git trees.

## Usage

```bash
bash scripts/fixtures_prepare.sh
bash scripts/fixtures_mutate_cases.sh
bash scripts/fixtures_verify.sh
```

Use `bash scripts/fixtures_bootstrap_poetry.sh --help` when you want explicit local Poetry environments for the prepared fixtures or generated cases. Bootstrap re-checks the installed Poetry environment and fails if `code-loader` resolves below `1.0.165`.

The scripts are fail-fast and stop on the first fixture error.
