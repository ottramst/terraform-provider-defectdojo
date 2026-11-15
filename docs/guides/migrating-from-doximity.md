---
page_title: "Migrate from doximity/defectdojo"
subcategory: ""
description: |-
  Move a configuration and its state from the unmaintained doximity/defectdojo provider to this provider, without recreating anything.
---

# Migrate from doximity/defectdojo

The [doximity/defectdojo](https://registry.terraform.io/providers/doximity/defectdojo) provider (last release v0.0.13, 2023) is
no longer maintained and was built for DefectDojo 2.x. This provider manages the same objects, under the same IDs:

| doximity/defectdojo | This provider | How the state moves |
| --- | --- | --- |
| `defectdojo_product_type` | `defectdojo_product_type` | Changing the provider source is enough |
| `defectdojo_product` | `defectdojo_product` | Changing the provider source is enough |
| `defectdojo_jira_product_configuration` | `defectdojo_jira_project` | A `moved` block |
| data sources `defectdojo_product_type`, `defectdojo_product` | the data sources of the same names | Nothing to move |

Nothing is recreated: the state keeps each object's DefectDojo ID, and the first plan reads everything else from DefectDojo.

## Before you start

- **DefectDojo 3.4 or 3.5.** This provider supports and tests the two newest DefectDojo minor releases. If you still run
  DefectDojo 2.x, upgrade DefectDojo first.
- **Terraform 1.8 or later** for the `moved` block of the Jira configuration. This provider's write-only attributes need
  Terraform 1.11, resource identity 1.12, and `terraform query` 1.14; it is tested with Terraform 1.16 and 1.17.
- **A backup of the state** (`terraform state pull > backup.tfstate`).

## 1. The provider configuration

Before:

```terraform
terraform {
  required_providers {
    defectdojo = {
      source  = "doximity/defectdojo"
      version = "0.0.13"
    }
  }
}

provider "defectdojo" {
  base_url = "https://defectdojo.example.com"
  api_key  = var.defectdojo_api_key
}
```

After:

```terraform
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
```

| doximity/defectdojo | This provider |
| --- | --- |
| `base_url`, `DEFECTDOJO_BASEURL` | `url`, `DEFECTDOJO_URL` |
| `api_key`, `DEFECTDOJO_APIKEY` | `api_token`, `DEFECTDOJO_API_TOKEN` |
| `username`, `password` (exchanged for an API token) | `username`, `password` (HTTP basic authentication, which also works when DefectDojo's API tokens are disabled); `DEFECTDOJO_USERNAME` and `DEFECTDOJO_PASSWORD` are the same |

## 2. The resources

Before:

```terraform
resource "defectdojo_product_type" "web" {
  name        = "Web applications"
  description = "Customer-facing web applications"
}

resource "defectdojo_product" "shop" {
  name                        = "Shop"
  description                 = "The web shop"
  product_type_id             = defectdojo_product_type.web.id
  business_criticality        = "high"
  platform                    = "web"
  life_cycle                  = "production"
  enable_skip_risk_acceptance = true
  user_records                = 25000
  revenue                     = "1000000.00"
  tags                        = ["pci"]
}

resource "defectdojo_jira_product_configuration" "shop" {
  jira_instance_id = "1"
  product_id       = defectdojo_product.shop.id
  project_key      = "SHOP"
}
```

After:

```terraform
resource "defectdojo_product_type" "web" {
  name        = "Web applications"
  description = "Customer-facing web applications"
}

resource "defectdojo_product" "shop" {
  name                 = "Shop"
  description          = "The web shop"
  product_type_id      = defectdojo_product_type.web.id
  business_criticality = "high"
  platform             = "web"
  life_cycle           = "production"
  # Renamed from enable_skip_risk_acceptance.
  enable_simple_risk_acceptance = true
  user_records                  = 25000
  revenue                       = "1000000.00"
  tags                          = ["pci"]
}

# Was defectdojo_jira_product_configuration.
resource "defectdojo_jira_project" "shop" {
  jira_instance_id = 1
  product_id       = defectdojo_product.shop.id
  project_key      = "SHOP"
}

moved {
  from = defectdojo_jira_product_configuration.shop
  to   = defectdojo_jira_project.shop
}
```

The changes, resource by resource:

- **All resources:** `id` is a number here, not a string.
- **`defectdojo_product_type`:**
  - `description = ""` meant "no description"; leave `description` out instead.
  - `authorized_users` and `force_destroy` are new.
- **`defectdojo_product`:**
  - `enable_skip_risk_acceptance` is `enable_simple_risk_acceptance`.
  - `enable_full_risk_acceptance` defaults to `true` here, as in DefectDojo, and to `false` there, although doximity/defectdojo
    left DefectDojo's `true` in place when it created a product without it. If the first plan changes it, set it to the value
    you want.
  - `""` in `business_criticality`, `platform`, `life_cycle` and `origin` meant "not set"; leave them out instead.
  - `revenue` must be written as DefectDojo returns it, with two decimal places: `"100.00"`, not `"100"`.
  - `prod_numeric_grade` is gone: DefectDojo calculates the grade from the findings.
  - `tags` and `regulation_ids`: there, leaving them out removed them all; here, leaving them out leaves them as they are, so
    that they can be managed elsewhere, and `[]` removes them all. Tags may be any text without spaces, commas or quotes, not
    only lowercase letters.
  - `authorized_users`, `sla_configuration_id`, `enable_product_tag_inheritance`, `disable_sla_breach_notifications` and
    `force_destroy` are new. `enable_product_tag_inheritance` and `disable_sla_breach_notifications` take DefectDojo's default
    (`false`) when they are left out: if they were turned on in DefectDojo's UI, add them to the configuration.
- **`defectdojo_jira_product_configuration` → `defectdojo_jira_project`:**
  - `jira_instance_id`, `product_id` and `engagement_id` are numbers.
  - `jira_instance_id` is required, and exactly one of `product_id` and `engagement_id` must be set.
  - `project_key = ""` meant "no key"; leave it out instead.
  - `component`, the custom and transition fields, `jira_labels`, `default_assignee`, `enabled`, `epic_issue_type_name` and
    the other settings DefectDojo has are new. Left out, they take DefectDojo's defaults (empty, or `enabled = true` and
    `epic_issue_type_name = "Epic"`). doximity/defectdojo couldn't manage them, so if they were set in DefectDojo's UI, the first
    plan shows them reset to those defaults, and applying it resets them: **add them to the configuration** first.
- **Data sources:**
  - `defectdojo_product_type` looks up by `id` or `name`; `defectdojo_product` too, where doximity/defectdojo's looked up by `id`
    only. Both return numeric IDs.
  - The product data source's `enable_skip_risk_acceptance` is `enable_simple_risk_acceptance`, and `prod_numeric_grade` is
    gone, as for the resource.

## 3. Switch

1. Make the changes of steps 1 and 2, including the `moved` block of every `defectdojo_jira_product_configuration`.
2. Run `terraform init -upgrade`. Terraform installs this provider; it still installs `doximity/defectdojo` too, because the state
   names it.
3. Run `terraform plan`. It should show the Jira configurations moving and nothing else; a change means the configuration and
   DefectDojo differ, as the list in step 2 explains. **Read it before applying**: a destroy or a replacement is a mistake
   (see [Troubleshooting](#troubleshooting)).
4. Run `terraform apply`, even when the plan shows no changes. The state now names this provider for every resource, with
   their resource identities, and `doximity/defectdojo` is no longer needed.

**Keep the addresses of product types and products during the switch.** A `moved` block can't rename them from
doximity/defectdojo: only the Jira configuration can move from that provider. Switch first, then rename them with an
ordinary `moved` block in a second change.

## What's new

- **Access:** DefectDojo 3.0 removed the role-based members of product types and products from its open-source edition; access
  is granted with `authorized_users` on product types and products. doximity/defectdojo had no resources for either.
- **Safe deletes:** destroying a product type, product or engagement deletes everything below it in DefectDojo, down to the
  findings, so this provider refuses unless `force_destroy` is set.
- **More resources:** engagements, SLA configurations, users, notifications, webhooks, system settings, Jira instances and the
  lookup objects. For the objects that already exist in DefectDojo, `terraform query` (Terraform 1.14+) finds them with this
  provider's list resources and generates `import` blocks and their configuration:

  ```terraform
  # objects.tfquery.hcl; run: terraform query -generate-config-out=generated.tf
  list "defectdojo_engagement" "shop" {
    provider = defectdojo

    config {
      product_id = 1
    }
  }
  ```

## Troubleshooting

- **Terraform asks for doximity/defectdojo's configuration** ("No base_url value provided and no DEFECTDOJO_BASEURL
  environment variable"): a resource in the state has no configuration, and Terraform wants doximity/defectdojo to destroy it.
  **Don't configure doximity/defectdojo to make the error go away**: destroying a product type deletes its products, their
  engagements and their findings, and doximity/defectdojo has no `force_destroy` guard. Instead:
  - a Jira configuration: add its `moved` block;
  - a product type or product whose address changed: restore its address, switch, then rename it (see step 3);
  - an object that Terraform should no longer manage: `terraform state rm` keeps it in DefectDojo.

  Configure doximity/defectdojo (with `DEFECTDOJO_BASEURL` and `DEFECTDOJO_APIKEY`, since the `provider "defectdojo"` block now
  configures this provider) only if the object should really be deleted.
- **The Jira configuration plans to be replaced or created:** its `moved` block is missing, or names another address.
- **`terraform plan -refresh=false`** after the move plans to replace the Jira project, which would delete it and its links
  between findings and Jira issues: the move only carries the ID, and the refresh fills in the rest. Don't apply that plan;
  plan with the refresh.
- **`terraform state replace-provider`** isn't needed: the first apply records the new provider for the product types and
  products. It can't move the Jira configurations, which need their `moved` blocks.
