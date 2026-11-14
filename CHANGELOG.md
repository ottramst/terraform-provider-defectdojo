## Unreleased

FEATURES:

* **New Resource:** `defectdojo_sla_configuration`, with resource identity. DefectDojo's Default SLA configuration can be imported; destroying it only removes it from the state ([#10](https://github.com/ottramst/terraform-provider-defectdojo/issues/10))
* **New Data Source:** `defectdojo_sla_configuration`, looked up by ID or name ([#10](https://github.com/ottramst/terraform-provider-defectdojo/issues/10))
* **New Resource:** `defectdojo_user`, with resource identity, configuration permissions by codename, and the write-only initial password `password_wo` ([#11](https://github.com/ottramst/terraform-provider-defectdojo/issues/11))
* **New Data Source:** `defectdojo_user`, looked up by ID or username ([#11](https://github.com/ottramst/terraform-provider-defectdojo/issues/11))
* **New Resource:** `defectdojo_notifications`, with resource identity: the channels of each event, system-wide, as the template for new users, per user, or per user and product ([#12](https://github.com/ottramst/terraform-provider-defectdojo/issues/12))
* **New Resource:** `defectdojo_notification_webhook`, with resource identity, for system notification webhooks; the header value is the write-only `header_value_wo`, sent when `header_value_wo_version` changes ([#13](https://github.com/ottramst/terraform-provider-defectdojo/issues/13))

ENHANCEMENTS:

* `defectdojo_product`: the explanation of DefectDojo's refusal to change an SLA configuration again while it recalculates SLA dates now also covers the days of SLA configurations ([#10](https://github.com/ottramst/terraform-provider-defectdojo/issues/10))

NOTES:

* `defectdojo_sla_configuration`: a change of the days or `enforce_*` attributes makes DefectDojo recalculate the SLA expiration dates of findings in the background; until its Celery workers have finished, another such change fails with an explanation, and so does a change of the SLA configuration of the products that use it. With the system setting `enable_finding_sla` off, DefectDojo never finishes ([#10](https://github.com/ottramst/terraform-provider-defectdojo/issues/10))
* `defectdojo_user`: DefectDojo's API can't change a password, so `password_wo` (Terraform 1.11+) is only sent when the user is created. `configuration_permissions` are codenames, such as `add_product_type`, which the provider maps to DefectDojo's IDs, and are only sent when they are configured; an empty set removes all the user's Django permissions ([#11](https://github.com/ottramst/terraform-provider-defectdojo/issues/11))
* `defectdojo_notification_webhook`: DefectDojo deactivates a webhook whose deliveries fail (`status`); a webhook it deactivates permanently can't be reactivated through the API: edit it in DefectDojo's UI or replace it ([#13](https://github.com/ottramst/terraform-provider-defectdojo/issues/13))
* `defectdojo_notifications`: DefectDojo creates the system-wide settings and each user's personal settings itself, so creating the resource adopts them; destroying them resets them to DefectDojo's defaults instead of deleting them, which would stop the user's notifications. Every event is managed: one that isn't configured gets DefectDojo's default, `alert`. `scan_added_empty` holds at most one channel, a limit of DefectDojo's API ([#12](https://github.com/ottramst/terraform-provider-defectdojo/issues/12))

## v0.1.0

FEATURES:

* **New Resource:** `defectdojo_product`, with resource identity and `force_destroy`, which guards against deleting the engagements of a product ([#7](https://github.com/ottramst/terraform-provider-defectdojo/issues/7))
* **New Data Source:** `defectdojo_product`, looked up by ID or name ([#7](https://github.com/ottramst/terraform-provider-defectdojo/issues/7))
* **New Resource:** `defectdojo_engagement`, with resource identity and `force_destroy`, which guards against deleting the tests of an engagement ([#8](https://github.com/ottramst/terraform-provider-defectdojo/issues/8))
* **New Data Source:** `defectdojo_engagement`, looked up by ID, or by product and name ([#8](https://github.com/ottramst/terraform-provider-defectdojo/issues/8))

NOTES:

* `defectdojo_engagement`: `status` and `engagement_type` are only sent when the configuration changes them, so that an engagement closed by a pipeline or by DefectDojo isn't reopened by an unrelated change; for such engagements, leave `status` unset, or use `lifecycle { ignore_changes = [status] }` with it. Imports into `CI/CD` engagements move `target_end` to the scan date. The tags of an engagement's product are hidden unless they are configured, because DefectDojo adds them when the product passes its tags on and doesn't say which tags are inherited ([#8](https://github.com/ottramst/terraform-provider-defectdojo/issues/8))
* `defectdojo_product`: `sla_configuration_id`, `authorized_users`, `regulation_ids` and `tags` are only sent when they are configured, so they can be managed elsewhere. Tags are compared ignoring case, because DefectDojo gives a tag the spelling of an existing tag that matches it ignoring case. The lifecycle is `life_cycle`, as in `doximity/defectdojo`, because `lifecycle` is reserved in Terraform ([#7](https://github.com/ottramst/terraform-provider-defectdojo/issues/7))
* Completes the v0.1.0 milestone (product types, products and engagements), built on go-defectdojo v0.1.0. Tested against DefectDojo 3.4.304 and 3.5.103 with Terraform 1.16 and 1.17

## v0.0.1

FEATURES:

* **New Resource:** `defectdojo_product_type`, with resource identity and `force_destroy`, which guards against deleting the products of a product type ([#4](https://github.com/ottramst/terraform-provider-defectdojo/issues/4))
* **New Data Source:** `defectdojo_product_type`, looked up by ID or name ([#4](https://github.com/ottramst/terraform-provider-defectdojo/issues/4))
* provider: authentication with an API v2 token, or a username and password ([#3](https://github.com/ottramst/terraform-provider-defectdojo/issues/3))

NOTES:

* First preview release, built on go-defectdojo v0.0.1. Products and engagements, the rest of the v0.1.0 milestone, follow
* Tested against DefectDojo 3.4.304 and 3.5.103 with Terraform 1.16 and 1.17
