# Terraform Provider for DefectDojo

[![CI/CD](https://github.com/ottramst/terraform-provider-defectdojo/actions/workflows/main.yml/badge.svg?branch=main)](https://github.com/ottramst/terraform-provider-defectdojo/actions/workflows/main.yml)
[![CodeQL](https://github.com/ottramst/terraform-provider-defectdojo/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/ottramst/terraform-provider-defectdojo/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/ottramst/terraform-provider-defectdojo/badge)](https://scorecard.dev/viewer/?uri=github.com/ottramst/terraform-provider-defectdojo)
[![Terraform Registry](https://img.shields.io/badge/terraform-registry-623CE4)](https://registry.terraform.io/providers/ottramst/defectdojo/latest)

This [Terraform](https://www.terraform.io) provider is built using the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) and allows you to manage the configuration of [OWASP DefectDojo](https://github.com/DefectDojo/django-DefectDojo), the open-source vulnerability management and DevSecOps platform, as code.

> [!NOTE]
> The provider is in early development (v0.1.0: product types, products and engagements), and its
> schema may still change before v1.0. See the [roadmap](#roadmap).

This provider includes:

- Resources and data sources for DefectDojo configuration objects (`internal/provider/`)
- Examples (`examples/`) and generated documentation (`docs/`)
- Acceptance tests against every supported DefectDojo version

It talks to DefectDojo through [go-defectdojo](https://github.com/ottramst/go-defectdojo), an idiomatic Go client for the DefectDojo API v2.

## Features

- **Product types**: Manage product types (DefectDojo 3.x *organizations*) and their authorized users, and look them up by ID or name
- **Products**: Manage products (DefectDojo 3.x *assets*) with their attributes, contacts, SLA configuration, regulations, tags and authorized users, and look them up by ID or name
- **Engagements**: Manage engagements, such as the long-lived CI/CD engagement a pipeline or Dependency-Track imports scans into, and look them up by ID, or by product and name. Tags a product passes on to its engagements are hidden, so they cause no drift
- **Safe deletes**: DefectDojo deletes everything below a deleted product type, product or engagement, down to findings; the provider refuses unless `force_destroy` is set
- **Adoption**: Import existing objects by ID or with an `import` block and resource identity (Terraform 1.12+)
- **Authentication**: An API v2 token, or a username and password when API tokens are disabled

SLA configurations, users, notifications, Jira, system settings and more are planned; see the [roadmap](#roadmap).

## DefectDojo version compatibility

Acceptance tests run in CI against **DefectDojo 3.4.304** and **3.5.103** (all legs are required to pass) across Terraform 1.16 and 1.17.

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

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0 (protocol 6); resource identity needs 1.12+. CI tests the non-EOL releases, currently 1.16 and 1.17 (see [support policy](#support-policy))
- [Go](https://golang.org/doc/install) >= 1.27 (only required to build the provider from source)
- [DefectDojo](https://github.com/DefectDojo/django-DefectDojo) 3.4.x or 3.5.x. Most configuration objects can only be managed by a superuser

## Using the Provider

```hcl
terraform {
  required_providers {
    defectdojo = {
      source  = "ottramst/defectdojo"
      version = "~> 0.1.0"
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
- [Example configurations](examples/)

### Migrating from doximity/defectdojo

The [doximity/defectdojo](https://registry.terraform.io/providers/doximity/defectdojo) provider is no longer maintained and targets DefectDojo 2.x. Moving its `defectdojo_product_type`, `defectdojo_product` and `defectdojo_jira_product_configuration` resources to this provider is planned, with `moved` blocks where possible. Until then, remove product types and products from state and import them here by their numeric IDs; the Jira product configuration follows with the Jira milestone (v0.3.0).

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
