# Contributing

Thanks for your interest in improving the DefectDojo Terraform provider. Bug reports, feature
requests and pull requests are all welcome. By participating you agree to follow the
[Code of Conduct](.github/CODE_OF_CONDUCT.md).

## Reporting issues

Use the [issue templates](https://github.com/ottramst/terraform-provider-defectdojo/issues/new/choose).
Always include the provider, Terraform and DefectDojo versions. Report security vulnerabilities
privately, as described in [SECURITY.md](SECURITY.md).

Planned work is tracked in [milestones](https://github.com/ottramst/terraform-provider-defectdojo/milestones):
each epic issue lists its resources as sub-issues. Comment on a sub-issue before starting on it, so
work isn't duplicated.

## Development setup

Requirements: [Go](https://go.dev/doc/install) (the version in `go.mod`), [Terraform](https://developer.hashicorp.com/terraform/install),
Docker, `jq`, and [golangci-lint](https://golangci-lint.run/) (the version pinned in `.github/workflows/lint.yml`).

```shell
make build      # build the provider
make test       # unit tests
make lint       # golangci-lint
make generate   # regenerate docs/ from the schemas and examples/
```

`make generate` must produce no diff in CI, so commit the regenerated `docs/` with every schema or
example change. Never edit `docs/` by hand: change the schema descriptions or `examples/` instead.

## The API client

The provider talks to DefectDojo through [go-defectdojo](https://github.com/ottramst/go-defectdojo), a
separate module. A resource that needs a new endpoint starts with a pull request to the client.

To develop both at once, check out go-defectdojo next to this repository and use a Go workspace,
which `.gitignore` keeps out of commits:

```shell
go work init . ../go-defectdojo
```

Once the client change is merged, require that commit of the client's `main` branch, never a commit of
a pull request branch (pull requests are squash-merged, so those commits disappear):

```shell
GOPROXY=direct go get github.com/ottramst/go-defectdojo@<commit on main>
go mod tidy
```

Never commit a `replace` directive. Before a provider release, the client must be tagged and required by
its version: the release workflow fails on a pseudo-version.

## Acceptance tests

Acceptance tests run against a real DefectDojo. A Docker Compose file starts PostgreSQL, Valkey and
DefectDojo in about 30 seconds; `DD_VERSION` selects the image tag and defaults to the newest
supported version.

```shell
make up
# make up DD_VERSION=3.4.304

make testacc
# or a single test:
make testacc TESTARGS='-run TestAccProductType'

make down
```

`make testacc` points the tests at the local stack's superuser (`admin` / `DefectDojoAcc1!` on
`http://localhost:8080`). Set `DEFECTDOJO_URL` and either `DEFECTDOJO_API_TOKEN` or
`DEFECTDOJO_USERNAME` and `DEFECTDOJO_PASSWORD` to run them against another DefectDojo; it needs a
superuser. The tests fail, rather than skip, when these are missing.

Behavior that differs between DefectDojo versions must be covered for each supported version. Run the
suite against every version in `.github/defectdojo-versions.json` before opening a pull request.

## Supported DefectDojo versions

CI tests the latest patch of the two newest DefectDojo minor releases (see the
[support policy](README.md#support-policy)). They are listed in one place,
`.github/defectdojo-versions.json`, oldest first, which the test matrix and `make up` read. The
`Upstream` workflow opens an issue when a new minor release is out. To support it:

1. Make sure go-defectdojo supports it (its contract test checks the API schema).
2. Update `.github/defectdojo-versions.json`: drop the oldest minor and add the new release. Set the
   `DD_VERSION` default in `docker-compose.yml` to the new release too; CI checks that they match.
3. Run the acceptance tests against both versions.
4. Update the tested versions in `README.md`, `templates/index.md.tmpl` (then `make generate`), and
   `CLAUDE.md`, and add a `NOTES` entry to the CHANGELOG. This is a minor release: DefectDojo versions moving
   forward don't break configurations (see [Stability](README.md#stability)). `TestAccUpgradeFromLatestRelease`
   skips DefectDojo versions the latest release wasn't tested on.

The same applies to Terraform: the `terraform-version` matrix in `.github/workflows/test.yml` covers the two
newest Terraform minor releases (the ones HashiCorp supports). When a new minor ships, add it, drop the oldest,
and update the versions in `README.md` and `CLAUDE.md`.

## Compatibility

From v1.0.0 a change that can break a configuration or a state of the latest release needs a new major
version; [Stability](README.md#stability) lists what counts. Two checks catch most of it:

- `TestSchemaCompatibility` (in `make test`) compares the schemas, identities, defaults and forced
  replacements with `internal/provider/testdata/schema-baseline.json`, the latest release's. Only a release
  pull request changes the baseline, with `go test ./internal/provider -run TestSchemaCompatibility -update`.
- `TestAccUpgradeFromLatestRelease` (acceptance) applies a configuration with the latest release from the
  Registry, then expects no changes with your build. Add a resource to its configuration when you add one.

What they can't see is up to review: a changed meaning of `null`, a validator that rejects what DefectDojo
accepts, or a `Read` that stores a different value. When DefectDojo removes a feature, deprecate the
attribute or resource (`DeprecationMessage`) instead of removing it.

## Pull requests

- Keep each pull request focused on one change, and add or update acceptance tests for it.
- Use a [Conventional Commits](https://www.conventionalcommits.org/) title, e.g.
  `feat: add defectdojo_product`; pull requests are squash-merged with that title.
- Add an entry to `CHANGELOG.md` under `## Unreleased` (`BREAKING CHANGES`, `FEATURES`, `ENHANCEMENTS`,
  `BUG FIXES` or `NOTES`).
- CI must pass: the `CI gate` check covers docs generation, lint, build, unit tests and the acceptance
  test matrix.

## Releases

Maintainers release by pushing a `vX.Y.Z` tag: a patch release for bug fixes, a minor release for new
features or DefectDojo versions. The release workflow checks that go-defectdojo is a tagged release, builds
and signs the artifacts with GoReleaser, attests their build provenance, and publishes a GitHub release that
the Terraform Registry picks up. Before tagging, in the release pull request (titled `chore: release vX.Y.Z`):

1. Move the `## Unreleased` entries in `CHANGELOG.md` under the new version.
2. Regenerate the schema baseline: `go test ./internal/provider -run TestSchemaCompatibility -update`.

After the release, the next pull request raises `upgradeFrom` and `upgradeFromDefectDojo` in
`internal/provider/upgrade_acc_test.go` to the new release.
