# Terraform Provider for DefectDojo

[![CI/CD](https://github.com/ottramst/terraform-provider-defectdojo/actions/workflows/main.yml/badge.svg?branch=main)](https://github.com/ottramst/terraform-provider-defectdojo/actions/workflows/main.yml)
[![CodeQL](https://github.com/ottramst/terraform-provider-defectdojo/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/ottramst/terraform-provider-defectdojo/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/ottramst/terraform-provider-defectdojo/badge)](https://scorecard.dev/viewer/?uri=github.com/ottramst/terraform-provider-defectdojo)
[![Terraform Registry](https://img.shields.io/badge/terraform-registry-623CE4)](https://registry.terraform.io/providers/ottramst/defectdojo/latest)

This [Terraform](https://www.terraform.io) provider is built using the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) and allows you to manage the configuration of [OWASP DefectDojo](https://github.com/DefectDojo/django-DefectDojo), the open-source vulnerability management and DevSecOps platform, as code.

> [!NOTE]
> The provider is in early development (v0.5.0: product types, products, engagements, SLA configurations,
> users, notifications, notification webhooks, system settings, Jira instances, Jira projects and the lookup objects:
> regulations, note types, development environments, the announcement, CI/CD infrastructure, product platforms,
> lifecycles and origins, and test types; list resources for `terraform query`; and migration from `doximity/defectdojo`),
> and its schema may still change before v1.0. See the [roadmap](#roadmap).

This provider includes:

- Resources, data sources and list resources for DefectDojo configuration objects (`internal/provider/`)
- Examples (`examples/`) and generated documentation (`docs/`)
- Acceptance tests against every supported DefectDojo version

It talks to DefectDojo through [go-defectdojo](https://github.com/ottramst/go-defectdojo), an idiomatic Go client for the DefectDojo API v2.

## Features

- **Product types**: Manage product types (DefectDojo 3.x *organizations*) and their authorized users, and look them up by ID or name
- **Products**: Manage products (DefectDojo 3.x *assets*) with their attributes, contacts, SLA configuration, regulations, tags and authorized users, and look them up by ID or name
- **SLA configurations**: Manage SLA configurations, the days in which findings of each severity must be remediated, including DefectDojo's Default one, and look them up by ID or name
- **Users**: Manage users, such as service accounts for pipelines and Dependency-Track, with their global configuration permissions by codename and a write-only initial password, and look them up by ID or username
- **Notifications**: Manage on which channels DefectDojo notifies of each event, system-wide, for new users, per user and per product; DefectDojo's own settings are adopted
- **Notification webhooks**: Manage system notification webhooks, with a write-only header value for their authentication
- **Jira instances**: Manage the connections to Jira servers, with a write-only password or API token, and look them up by ID or name
- **Jira projects**: Connect products and engagements to Jira projects, with custom and transition fields as JSON
- **CI/CD infrastructure**: Manage the SCM servers, build servers and orchestration engines that engagements link to, and look them up by ID or name
- **Announcement**: Manage the banner DefectDojo shows every user; a new message replaces it, so everyone sees it again
- **Development environments**: Manage the environments tests run in, and look them up by ID or name
- **Note types**: Manage the types of notes on findings, tests and engagements; a note type with notes is only destroyed with `force_destroy`, since DefectDojo deletes its notes with it
- **Product platforms, lifecycles and origins**: Manage the values products use in `platform`, `life_cycle` and `origin`
- **Regulations**: Manage regulations, such as NIS2, and look up the ones DefectDojo comes with by ID, name or acronym for a product's `regulation_ids`
- **Test types**: List the test types, such as the scanners DefectDojo can import, for example to check a scan type in a `check` block
- **System settings**: Manage the settings of the whole instance, such as deduplication, SLA notifications and the notification channels; only the settings in the configuration are changed, and the secrets are write-only
- **Engagements**: Manage engagements, such as the long-lived CI/CD engagement a pipeline or Dependency-Track imports scans into, and look them up by ID, or by product and name. Tags a product passes on to its engagements are hidden, so they cause no drift
- **Safe deletes**: DefectDojo deletes everything below a deleted product type, product or engagement, down to findings; the provider refuses unless `force_destroy` is set
- **Adoption**: Import existing objects by ID or with an `import` block and resource identity (Terraform 1.12+), or find them with the list resources of `terraform query` (Terraform 1.14+), which generates `import` blocks and their configuration
- **Migration from doximity/defectdojo**: Take over its product types, products and Jira product configurations without recreating them; see the [migration guide](docs/guides/migrating-from-doximity.md)
- **Authentication**: An API v2 token, or a username and password when API tokens are disabled

More is planned; see the [roadmap](#roadmap).

## Stability

From v1.0.0, the provider follows [semantic versioning](https://semver.org/): a configuration and a state
that work with a v1 release keep working with later v1 releases. A change that can break them needs v2.0.0:

- **Breaking:** removing or renaming a resource, data source, list resource or attribute; changing an
  attribute's type; making an attribute required, or one that could be set computed-only; changing a default
  or what `null` means for an attribute (such as `authorized_users`, left as it is when unset, versus
  `lead_id`, cleared when unset); a change that newly forces replacement; changing a resource identity.
- **Not breaking:** new resources, data sources, list resources and optional attributes; deprecations; a new
  validator that rejects only values DefectDojo rejects anyway; fixes that make the provider match DefectDojo.

CI enforces this: every pull request compares the schemas with the latest release's, and applies a
configuration with the latest release before planning it with the new build, which must plan no changes.

**When DefectDojo removes a feature** in a minor release, the provider doesn't break configurations on the
versions that still have it: an attribute whose field DefectDojo removes is deprecated (Terraform warns when it
is set) and kept until v2; a resource whose endpoint DefectDojo removes is deprecated, and fails with
DefectDojo's error on versions without it. Values DefectDojo itself defines, such as the codenames of
`configuration_permissions`, follow DefectDojo: the `*_tool_type` and `*_tool_configuration` permissions,
which DefectDojo 3.6 removes, fail with the existing "unknown codename" error on 3.6.

## DefectDojo version compatibility

Acceptance tests run in CI against **DefectDojo 3.4.304** and **3.5.103** (all legs are required to pass) across Terraform 1.16 and 1.17. Moving the tested DefectDojo versions forward is a minor release of the provider.

### Support policy

DefectDojo ships a minor release every month and patches every week, and supports only its latest release. The provider fully supports, and tests the latest patch of, the two newest DefectDojo minor releases:

| Release line | Provider support |
| --- | --- |
| 3.5.x | Supported, tested in CI |
| 3.4.x | Supported, tested in CI |
| 3.0.x to 3.3.x | Best effort |
| 2.x | Not supported |

*Best effort* means the provider may still work, but these versions are not tested in CI, and a bug that only reproduces on them may not be fixed. DefectDojo 2.x is not supported: 3.0 removed the role-based access endpoints (groups, members and roles) from the open-source edition, and access is now granted with `authorized_users`. DefectDojo Pro features are not tested.

A nightly canary job runs the acceptance tests against DefectDojo's development build, and a weekly job opens an issue when a new minor release is out.

Terraform follows the same policy. HashiCorp supports the two newest Terraform minor releases, currently **1.17** and **1.16**, and CI tests those. The provider uses plugin protocol 6, so it still works with older Terraform 1.x releases, but EOL Terraform versions are supported on a best-effort basis only.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) 1.11 or later, which the write-only attributes (`password_wo`, `header_value_wo`, the system settings' secrets) need. Import by resource identity needs 1.12, `terraform query` 1.14, and the `moved` block of the doximity migration 1.8. CI tests the non-EOL releases, currently 1.16 and 1.17; older releases are untested (see [support policy](#support-policy))
- [Go](https://golang.org/doc/install) >= 1.27 (only required to build the provider from source)
- [DefectDojo](https://github.com/DefectDojo/django-DefectDojo) 3.4.x or 3.5.x, tested in CI; other 3.x releases get best-effort support (see [support policy](#support-policy)). Most configuration objects can only be managed by a superuser

## Using the Provider

```hcl
terraform {
  required_providers {
    defectdojo = {
      source  = "ottramst/defectdojo"
      version = "~> 0.5.0"
    }
  }
}

provider "defectdojo" {
  url       = "https://defectdojo.example.com"
  api_token = var.defectdojo_api_token
}

resource "defectdojo_product_type" "web" {
  name             = "Web Applications"
  description      = "Customer-facing web applications"
  critical_product = true
}
```

For details, see the:
- [Provider documentation](docs/index.md) for authentication and environment variables
- [Resource documentation](docs/resources/) for available resources
- [Data source documentation](docs/data-sources/) for available data sources
- [List resource documentation](docs/list-resources/) for `terraform query`
- [Example configurations](examples/)
- [Guides](docs/guides/), such as connecting [Dependency-Track](docs/guides/dependency-track.md) and [migrating from doximity/defectdojo](docs/guides/migrating-from-doximity.md)

### Migrating from doximity/defectdojo

The [doximity/defectdojo](https://registry.terraform.io/providers/doximity/defectdojo) provider is no longer maintained and targets DefectDojo 2.x. Its state moves here without recreating anything:

- `defectdojo_product_type` and `defectdojo_product` have the same names here: change the provider source in `required_providers` to `ottramst/defectdojo`, and this provider takes over their state.
- `defectdojo_jira_product_configuration` is `defectdojo_jira_project` here: move it with a `moved` block (Terraform 1.8+).

Some attributes were renamed or changed meaning, so the configuration needs updating before the first plan. The [migration guide](docs/guides/migrating-from-doximity.md) walks through it.

## Roadmap

The path to v1.0 is tracked in [milestones](https://github.com/ottramst/terraform-provider-defectdojo/milestones) and on the [DefectDojo roadmap board](https://github.com/users/ottramst/projects/4). Each epic issue lists its resources as sub-issues, and each resource builds on a [go-defectdojo](https://github.com/ottramst/go-defectdojo) service. Contributions are welcome.

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`. To develop against a local checkout of go-defectdojo, see [CONTRIBUTING.md](CONTRIBUTING.md#the-api-client).

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `make generate`.

### Running Acceptance Tests

Acceptance tests require a running DefectDojo. A `docker-compose.yml` is provided; `DD_VERSION` selects the image tag and defaults to the newest supported version.

```shell
# Start a local DefectDojo on http://localhost:8080 (about 30 seconds)
make up                         # or: make up DD_VERSION=3.4.304

make testacc

make down
```

`make testacc` uses the local stack's superuser; set `DEFECTDOJO_URL` and either `DEFECTDOJO_API_TOKEN` or `DEFECTDOJO_USERNAME` and `DEFECTDOJO_PASSWORD` to use another DefectDojo.

*Note:* Acceptance tests create and delete real objects. Use a test instance, not a production DefectDojo.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MPL-2.0](LICENSE)
