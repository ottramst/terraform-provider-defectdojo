# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Terraform provider for [OWASP DefectDojo](https://github.com/DefectDojo/django-DefectDojo), built with the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework). Provider code lives in `internal/provider/`. The API client is a separate module, [go-defectdojo](https://github.com/ottramst/go-defectdojo) (same author, MIT): a resource that needs an endpoint the client lacks starts with a pull request there. DefectDojo API behavior is documented in go-defectdojo's `CLAUDE.md` ("DefectDojo API quirks"); keep new findings there.

The provider supports and tests the latest patch of the two newest DefectDojo minor releases (currently 3.4.304 and 3.5.103), listed once in `.github/defectdojo-versions.json`. DefectDojo 2.x is not supported: 3.0 removed the role-based access endpoints (groups, members, roles) from the open-source edition, and access is granted with `authorized_users` on product types and products. Most configuration endpoints need a superuser.

Scope is DefectDojo's configuration, planned as milestones: product types, products and engagements (v0.1.0); SLA configurations, users, notifications and webhooks (v0.2.0); system settings and Jira (v0.3.0); lookup objects (v0.4.0); list resources and migration from `doximity/defectdojo` (v0.5.0). Runtime data (findings, tests, imports) is out of scope. Endpoints removed in DefectDojo 3.6 (tool types, tool configurations, API scan configurations) are not supported.

## Common Commands

```bash
make default          # fmt, lint, install, generate (full dev cycle)
make build            # Build binary
make lint             # Run golangci-lint
make fmt              # Format Go code with gofmt
make generate         # Generate provider documentation (tfplugindocs) + terraform fmt examples
make test             # Unit tests (120s timeout, parallel=10)
make up               # Start DefectDojo (DD_VERSION defaults to the newest supported)
make testacc          # Acceptance tests against it
make down             # Stop it and delete its data
```

Run a single test:
```bash
make testacc TESTARGS='-run TestAccProductType'
```

## Running Acceptance Tests Locally

```bash
make up                        # DefectDojo 3.5.103 on http://localhost:8080, about 30 seconds
# make down && make up DD_VERSION=3.4.304
make testacc
make down
```

The compose stack (shared with go-defectdojo) is PostgreSQL (tmpfs), Valkey, a one-shot `initializer`, `uwsgi`, and DefectDojo's `nginx` on `127.0.0.1:8080`; Celery is left out, and deletes are synchronous. The `GNUmakefile` exports `DEFECTDOJO_URL`, `DEFECTDOJO_USERNAME` (`admin`) and `DEFECTDOJO_PASSWORD` (`DefectDojoAcc1!`) for the tests unless they are set. `testAccPreCheck` fails the test, never skips it, when they are missing. The initializer loads fixtures: the product type "Research and Development" (id 1), the "Default" SLA configuration, development environments, regulations and test types.

## Architecture

**Provider entry point:** `main.go` → `internal/provider/provider.go`

`Configure()` builds a `*defectdojo.Client` from `url` and `api_token`, or `username` and `password` (basic auth, which also works when API tokens are disabled; the token wins when both are set), each falling back to a `DEFECTDOJO_*` environment variable. There is no version probe: DefectDojo's only version source is its 860 KB OpenAPI schema. The client is passed to every resource and data source (`configureResource`, `configureDataSource` in `helpers.go`).

**Resource pattern** (all files in `internal/provider/`; `resource_product_type.go` is the template):
- Files are `resource_<name>.go` and `data_source_<name>.go`; constructors are `New<Name>Resource` and `New<Name>DataSource`, registered in `Resources()` and `DataSources()`.
- Each resource has a struct holding the `*defectdojo.Client` and a model struct with `tfsdk` tags. IDs are DefectDojo's integer IDs, as `types.Int64`.
- `Read()` removes the resource from state on `defectdojo.ErrNotFound`; deletes of already-deleted objects succeed.
- Errors use descriptive titles: `"Error creating DefectDojo product type"`, with the object ID in the detail.
- Optional text attributes that DefectDojo stores as `""` or null map to null (`stringOrNull`) and reject `""`, so there is no perpetual diff.
- DefectDojo strips leading and trailing whitespace from text fields (Python `str.strip`, Unicode-aware), which would make apply fail with "inconsistent result" (a heredoc ends in `\n`): text attributes use `noSurroundingWhitespace()`. Lengths are characters, not bytes: use `stringvalidator.UTF8LengthBetween`.
- `Update()` uses the client's `Patch` methods with `Nullable` fields, so attributes the configuration doesn't manage are kept. Optional+Computed attributes that others may manage (such as `authorized_users`) use `UseStateForUnknown` and are only sent when the **configuration** sets them: the plan holds the prior state value when they are not configured, so read `req.Config`, not the plan. `resource_product_type_requests_test.go` checks the request bodies against a fake DefectDojo.
- **Cascading deletes:** deleting a product type, product or engagement deletes everything below it, down to findings. Such resources have `force_destroy` (default `false`): `Delete()` counts the children first and refuses with an error naming them. After the DELETE, `waitForDeletion` polls until the object is gone, for servers with `DD_ASYNC_OBJECT_DELETE`.
- **Resource identity:** every resource implements `resource.ResourceWithIdentity` with the identity `id` (`idIdentitySchema`, `setIDIdentity`), set in `Create`, `Read` and `Update`. `ImportState` uses `importByID`, which accepts a numeric import ID or the identity. Changing an identity schema needs an identity upgrader and an upgrade test.
- Data sources look up by `id` or exact `name` (`datasourcevalidator.ExactlyOneOf`), using the client's `GetByName`, which re-checks the name on the client side.

## Test Patterns

Acceptance tests require a real DefectDojo. Tests follow this structure:
- `TestAcc<Resource>(t)`: create with defaults, update every attribute, import by ID, import by identity (`ImportBlockWithResourceIdentity`), changed outside Terraform (reverted), deleted outside Terraform (re-created). Plus tests for guards such as `force_destroy`.
- Configs omit the `provider` block: the provider reads the environment.
- `testAccClient(t)` returns a go-defectdojo client configured from the environment, to change or check objects in `PreConfig` and check functions. Objects the client cannot create yet are created with `client.NewRequest`/`Do`.
- Assertions use `statecheck.ExpectKnownValue` with `knownvalue`, `statecheck.CompareValuePairs`, `statecheck.ExpectIdentityValueMatchesState` and `plancheck.ExpectResourceAction`.
- Destroy after a test uses the last step's config, so end with a valid config after `ExpectError` steps.
- A regression test must fail without its fix: when two resources change in the same step, Terraform applies them in parallel, which can mask a bug. Change them in separate steps.

## The client dependency

`go.mod` requires go-defectdojo by a pseudo-version of a commit on its `main` branch while a feature is in development (`GOPROXY=direct go get github.com/ottramst/go-defectdojo@<sha>`), never a commit of a pull request branch, which squash merges discard. Never commit a `replace` directive; `go.work` (gitignored) is for local development. `release.yml` fails when go-defectdojo is a pseudo-version: tag the client first.

## CI

GitHub Actions workflows in `.github/workflows/` (`main.yml` orchestrates `generate`, `lint`, `build`, `test`, and a single `CI gate` job used as the required status check):
- Acceptance tests run a matrix of DefectDojo (from `.github/defectdojo-versions.json`; a `versions` job also checks that the `DD_VERSION` default in `docker-compose.yml` is the newest) × Terraform {1.16.\*, 1.17.\*} (the two non-EOL minors)
- `upstream.yml` (scheduled, not in the CI gate): a nightly canary on DefectDojo's `nightly-dev` image, and a weekly job that opens an issue when a new DefectDojo minor is released
- All `setup-go` steps use `go-version-file: go.mod`, and all actions are pinned to commit SHAs
- `make generate` must produce no git diff, and `go.mod`/`go.sum` must be tidy
- golangci-lint enforced with zero tolerance (`max-issues-per-linter: 0`)
- Also: CodeQL, govulncheck and dependency review (`security.yml`), OpenSSF Scorecard, and actionlint plus zizmor for workflow changes
- `release.yml`: on a `v*` tag, in the `release` environment, GoReleaser builds and GPG-signs the artifacts (secrets `GPG_PRIVATE_KEY` and `PASSPHRASE`) and attests their provenance

## Rules

- Add a `CHANGELOG.md` entry under `## Unreleased` for user-visible changes (HashiCorp style: `BREAKING CHANGES`, `FEATURES`, `ENHANCEMENTS`, `BUG FIXES`, `NOTES`), linking the issue.
- Every resource needs examples (`resource.tf`, `import.sh`, `import-by-identity.tf`), regenerated docs (`make generate`), and acceptance tests that pass on every supported DefectDojo version.
- Pull request titles follow Conventional Commits (`feat: add defectdojo_product`). PRs are squash-merged and reference their issue (`Closes #N`).
