# Concierge Release Protocol

This repository uses semantic version tags (`vX.Y.Z`) and GitHub Actions to publish binaries and release notes.

## Versioning Rules

Choose the next version manually using Semantic Versioning:

- `MAJOR` (`v2.0.0`): backward-incompatible CLI or behavior changes.
- `MINOR` (`v1.4.0`): backward-compatible new functionality.
- `PATCH` (`v1.4.3`): backward-compatible fixes only.

## Automated Release Flow (Recommended)

1. Ensure `main` is green in CI.
2. Open GitHub Actions and run the `Release` workflow.
3. Enter `version` as either `X.Y.Z` or `vX.Y.Z`.
4. The workflow will:
   - validate semver,
   - create and push annotated tag `vX.Y.Z` on latest `main` (or reuse existing tag if it already exists),
   - run `go test ./...`,
   - build and publish Linux/macOS binaries for `amd64` and `arm64`,
   - publish GitHub release notes via GoReleaser.

## Re-run / Recovery

- If a release fails after tag creation, run `Release` again with the same `version`; the workflow reuses that existing tag and retries publication.

## Links

- Releases list: `https://github.com/tensorleap/concierge/releases`
- Latest release page: `https://github.com/tensorleap/concierge/releases/latest`
