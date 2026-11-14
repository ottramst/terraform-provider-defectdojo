## Unreleased

FEATURES:

* **New Resource:** `defectdojo_product`, with resource identity and `force_destroy`, which guards against deleting the engagements of a product ([#7](https://github.com/ottramst/terraform-provider-defectdojo/issues/7))
* **New Data Source:** `defectdojo_product`, looked up by ID or name ([#7](https://github.com/ottramst/terraform-provider-defectdojo/issues/7))
* **New Resource:** `defectdojo_engagement`, with resource identity and `force_destroy`, which guards against deleting the tests of an engagement ([#8](https://github.com/ottramst/terraform-provider-defectdojo/issues/8))
* **New Data Source:** `defectdojo_engagement`, looked up by ID, or by product and name ([#8](https://github.com/ottramst/terraform-provider-defectdojo/issues/8))

NOTES:

* `defectdojo_engagement`: `status` and `engagement_type` are only sent when the configuration changes them, so that an engagement closed by a pipeline or by DefectDojo isn't reopened by an unrelated change; for such engagements, leave `status` unset, or use `lifecycle { ignore_changes = [status] }` with it. Imports into `CI/CD` engagements move `target_end` to the scan date. The tags of an engagement's product are hidden unless they are configured, because DefectDojo adds them when the product passes its tags on and doesn't say which tags are inherited ([#8](https://github.com/ottramst/terraform-provider-defectdojo/issues/8))
* `defectdojo_product`: `sla_configuration_id`, `authorized_users`, `regulation_ids` and `tags` are only sent when they are configured, so they can be managed elsewhere. Tags are compared ignoring case, because DefectDojo gives a tag the spelling of an existing tag that matches it ignoring case. The lifecycle is `life_cycle`, as in `doximity/defectdojo`, because `lifecycle` is reserved in Terraform ([#7](https://github.com/ottramst/terraform-provider-defectdojo/issues/7))

## v0.0.1

FEATURES:

* **New Resource:** `defectdojo_product_type`, with resource identity and `force_destroy`, which guards against deleting the products of a product type ([#4](https://github.com/ottramst/terraform-provider-defectdojo/issues/4))
* **New Data Source:** `defectdojo_product_type`, looked up by ID or name ([#4](https://github.com/ottramst/terraform-provider-defectdojo/issues/4))
* provider: authentication with an API v2 token, or a username and password ([#3](https://github.com/ottramst/terraform-provider-defectdojo/issues/3))

NOTES:

* First preview release, built on go-defectdojo v0.0.1. Products and engagements, the rest of the v0.1.0 milestone, follow
* Tested against DefectDojo 3.4.304 and 3.5.103 with Terraform 1.16 and 1.17
