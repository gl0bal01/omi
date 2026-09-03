# Release Checklist

Use this checklist for every tagged `omi` release.

## Preflight

- Confirm `CHANGELOG.md` has an entry for the release version and date.
- Confirm `cmd/omi/main.go` default `version` matches `Makefile VERSION` or that release ldflags set it explicitly.
- Run `make verify` from a clean checkout.
- Run `scripts/sync-models.sh --strict` with network access and triage any chat/code catalog drift.
- If a real API key is available, run `OMI_API_KEY=... BIN=./bin/omi scripts/smoke.sh` after `make build`.

## Version And Tag

```sh
VERSION=0.3.0 make verify
git tag v0.3.0
git push origin v0.3.0
```

## Artifact Verification

Local snapshot artifacts are reproducible with:

```sh
goreleaser release --snapshot --clean
```

Expected output paths:

- `bin/omi` from `make build`.
- `dist/omi_<os>_<arch>*/omi` or `omi.exe` from GoReleaser builds.
- `dist/omi-<version>-<os>-<arch>.tar.gz` for Linux/macOS archives.
- `dist/omi-<version>-windows-amd64.zip` for Windows archives.
- `dist/SHA256SUMS` for release checksums.

Verify each archive extracts and runs:

```sh
./bin/omi --version
sha256sum -c dist/SHA256SUMS
```

## Release Gate

Release is a go only if all required gates pass:

- `make verify` passes.
- `go test ./...` passes.
- `scripts/sync-models.sh --strict` passes or catalog drift is documented in the changelog.
- Smoke testing passes, or the release notes explicitly state that live smoke was skipped because no API key was available.
- GitHub Actions CI is green on the release commit.
