## Unreleased

FEATURES:

* **New List Resources:** `defectdojo_product_type`, `defectdojo_product`, `defectdojo_engagement` and `defectdojo_user`, for `terraform query` (Terraform 1.14+): find existing objects, optionally filtered by name (`name_contains`, `username_contains`, ignoring case), product type or product, and generate `import` blocks and configuration for them with `terraform query -generate-config-out`. The generated configuration plans no changes; it sets every attribute that has a value, so remove those another system manages ([#28](https://github.com/ottramst/terraform-provider-defectdojo/issues/28))
* **New List Resources:** `defectdojo_sla_configuration`, `defectdojo_jira_instance`, `defectdojo_jira_project`, `defectdojo_notification_webhook`, `defectdojo_notifications`, `defectdojo_regulation`, `defectdojo_note_type`, `defectdojo_development_environment`, `defectdojo_cicd_infrastructure`, `defectdojo_product_platform`, `defectdojo_product_lifecycle` and `defectdojo_product_origin`, for `terraform query`, so that every resource except the singletons (`defectdojo_system_settings` and `defectdojo_announcement`) has one. The generated configuration of a Jira instance lacks the required `password_wo`, which DefectDojo never returns ([#65](https://github.com/ottramst/terraform-provider-defectdojo/issues/65))
* **Migration from `doximity/defectdojo`:** `defectdojo_jira_project` accepts a `moved` block from its `defectdojo_jira_product_configuration`, and `defectdojo_product_type` and `defectdojo_product` take over its state when the provider source changes, since their names are the same. Only the ID is moved; the plan then reads the rest from DefectDojo ([#29](https://github.com/ottramst/terraform-provider-defectdojo/issues/29))

ENHANCEMENTS:

* `defectdojo_product`: the `user_records` and `revenue` attributes, the estimated number of user records and the estimated revenue, which `doximity/defectdojo` also manages; the `defectdojo_product` data source has them too. `revenue` is written with two decimal places, as DefectDojo returns it (`"100.00"`) ([#64](https://github.com/ottramst/terraform-provider-defectdojo/issues/64))

BUG FIXES:

* Planning failed with "Missing Resource Identity After Read" when an object had been deleted outside Terraform and the state had no resource identity: a state taken over from `doximity/defectdojo`, or one used with Terraform before 1.12, which doesn't send identities. The resource is now removed from the state and planned for creation ([#63](https://github.com/ottramst/terraform-provider-defectdojo/issues/63))

NOTES:

* `defectdojo_product`: like the other optional attributes, `user_records` and `revenue` are cleared when they aren't configured. After upgrading, the next plan shows an update for every product whose user records or revenue were set outside Terraform, which clears them; add them to the configuration before applying it to keep them ([#64](https://github.com/ottramst/terraform-provider-defectdojo/issues/64))

## v0.4.0

FEATURES:

* **New Resource:** `defectdojo_announcement`, with resource identity: DefectDojo's banner for every user. A change of `message` or `style` replaces it, as DefectDojo's UI does, so that users who dismissed the old one see the new one ([#23](https://github.com/ottramst/terraform-provider-defectdojo/issues/23))
* **New Resource:** `defectdojo_cicd_infrastructure`, with resource identity: a source code management server, build server or orchestration engine. Changing its type replaces it, since DefectDojo can't change it ([#24](https://github.com/ottramst/terraform-provider-defectdojo/issues/24))
* **New Data Source:** `defectdojo_cicd_infrastructure`, looked up by ID, or by name and type ([#24](https://github.com/ottramst/terraform-provider-defectdojo/issues/24))
* **New Resource:** `defectdojo_development_environment`, with resource identity. A name that another environment has is refused, since it would break the imports that name it, and an environment that tests use can't be destroyed ([#22](https://github.com/ottramst/terraform-provider-defectdojo/issues/22))
* **New Data Source:** `defectdojo_development_environment`, looked up by ID or name; a name that several environments have is an error ([#22](https://github.com/ottramst/terraform-provider-defectdojo/issues/22))
* **New Resource:** `defectdojo_note_type`, with resource identity. DefectDojo deletes the notes of a note type with it, so the provider refuses to destroy a note type that has notes unless `force_destroy` is set ([#21](https://github.com/ottramst/terraform-provider-defectdojo/issues/21))
* **New Data Source:** `defectdojo_note_type`, looked up by ID or name ([#21](https://github.com/ottramst/terraform-provider-defectdojo/issues/21))
* **New Resources:** `defectdojo_product_platform`, `defectdojo_product_lifecycle` and `defectdojo_product_origin`, with resource identity: the values products use in `platform`, `life_cycle` and `origin`. Changing `value` replaces the entry; set `create_before_destroy` when products use it ([#25](https://github.com/ottramst/terraform-provider-defectdojo/issues/25))
* **New Resource:** `defectdojo_regulation`, with resource identity. Destroying a regulation removes it from the products that list it ([#20](https://github.com/ottramst/terraform-provider-defectdojo/issues/20))
* **New Data Source:** `defectdojo_regulation`, looked up by ID, name or acronym, such as the regulations DefectDojo comes with ([#20](https://github.com/ottramst/terraform-provider-defectdojo/issues/20))
* **New Data Source:** `defectdojo_test_types`: the test types, such as the scanners DefectDojo can import, optionally filtered by name ([#26](https://github.com/ottramst/terraform-provider-defectdojo/issues/26))

ENHANCEMENTS:

* `defectdojo_engagement`: the `cicd_scm_server_id`, `cicd_build_server_id` and `cicd_orchestration_engine_id` attributes link an engagement to its CI/CD infrastructure; the `defectdojo_engagement` data source has them too ([#24](https://github.com/ottramst/terraform-provider-defectdojo/issues/24))

NOTES:

* `defectdojo_engagement`: like `lead_id`, the new CI/CD links are cleared when they aren't configured. After upgrading, the next plan shows an update for every engagement whose CI/CD links were set outside Terraform, which clears them; add the links to the configuration before applying it to keep them ([#24](https://github.com/ottramst/terraform-provider-defectdojo/issues/24))
* Completes the v0.4.0 milestone (lookup objects), built on go-defectdojo v0.4.0. Tested against DefectDojo 3.4.304 and 3.5.103, with Terraform 1.16 and 1.17

## v0.3.0

FEATURES:

* **New Resource:** `defectdojo_system_settings`, with resource identity: DefectDojo's system settings. Only the configured settings are changed, the others are read; the Slack token, the Microsoft Teams webhook URL and the Jira webhook secret are write-only attributes with a version; destroying the resource leaves the settings as they are ([#16](https://github.com/ottramst/terraform-provider-defectdojo/issues/16))
* **New Data Source:** `defectdojo_system_settings`, with every setting but the secrets, the installation ID and the read-only mode ([#16](https://github.com/ottramst/terraform-provider-defectdojo/issues/16))
* **New Resource:** `defectdojo_jira_instance`, with resource identity: the connection to a Jira server, with the write-only `password_wo` ([#17](https://github.com/ottramst/terraform-provider-defectdojo/issues/17))
* **New Data Source:** `defectdojo_jira_instance`, looked up by ID or name ([#17](https://github.com/ottramst/terraform-provider-defectdojo/issues/17))
* **New Resource:** `defectdojo_jira_project`, with resource identity: the Jira project of a product or an engagement, with its custom and transition fields as JSON ([#18](https://github.com/ottramst/terraform-provider-defectdojo/issues/18))

NOTES:

* `defectdojo_jira_project`: it needs edit permission on the product or engagement, not a superuser, and doesn't need the system setting `enable_jira`. The provider refuses to create a second Jira project for a product, which DefectDojo would allow but ignore. `suppress_sla_notification_comments` needs DefectDojo 3.5.103 or later. Destroying it deletes the links between findings and Jira issues made through it ([#18](https://github.com/ottramst/terraform-provider-defectdojo/issues/18))
* `defectdojo_jira_instance`: the password or API token is sent when the instance is created, when `password_wo_version` changes, and when the URL changes, which DefectDojo allows only with it. A delete is refused while Jira projects use the instance, which DefectDojo would answer with a server error ([#17](https://github.com/ottramst/terraform-provider-defectdojo/issues/17))
* `defectdojo_system_settings`: removing a setting from the configuration leaves its value in DefectDojo, and settings DefectDojo can hold as null (such as `max_dupes`) can't be set to null; the secrets can't be read back, and removing them leaves their values (the Slack token and the Teams URL are cleared with `""` and a new version; the Jira webhook secret can't be cleared). DefectDojo's read-only mode is left out on purpose: while it is on, DefectDojo rejects every other change ([#16](https://github.com/ottramst/terraform-provider-defectdojo/issues/16))
* Completes the v0.3.0 milestone (system settings, Jira instances and Jira projects), built on go-defectdojo v0.3.0. Tested against DefectDojo 3.4.304 and 3.5.103 with Terraform 1.16 and 1.17

## v0.2.0

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

* The provider is built with Go 1.27.4: v0.1.0 was built with Go 1.27.0, whose standard library has vulnerabilities in `net/http`, `crypto/tls` and `crypto/x509` that the provider's HTTP client reaches. `go.mod`'s `toolchain` line now pins the Go of CI and releases, and govulncheck checks it
* `defectdojo_sla_configuration`: a change of the days or `enforce_*` attributes makes DefectDojo recalculate the SLA expiration dates of findings in the background; until its Celery workers have finished, another such change fails with an explanation, and so does a change of the SLA configuration of the products that use it. With the system setting `enable_finding_sla` off, DefectDojo never finishes ([#10](https://github.com/ottramst/terraform-provider-defectdojo/issues/10))
* `defectdojo_user`: DefectDojo's API can't change a password, so `password_wo` (Terraform 1.11+) is only sent when the user is created. `configuration_permissions` are codenames, such as `add_product_type`, which the provider maps to DefectDojo's IDs, and are only sent when they are configured; an empty set removes all the user's Django permissions ([#11](https://github.com/ottramst/terraform-provider-defectdojo/issues/11))
* `defectdojo_notification_webhook`: DefectDojo deactivates a webhook whose deliveries fail (`status`); a webhook it deactivates permanently can't be reactivated through the API: edit it in DefectDojo's UI or replace it ([#13](https://github.com/ottramst/terraform-provider-defectdojo/issues/13))
* `defectdojo_notifications`: DefectDojo creates the system-wide settings and each user's personal settings itself, so creating the resource adopts them; destroying them resets them to DefectDojo's defaults instead of deleting them, which would stop the user's notifications. Every event is managed: one that isn't configured gets DefectDojo's default, `alert`. `scan_added_empty` holds at most one channel, a limit of DefectDojo's API ([#12](https://github.com/ottramst/terraform-provider-defectdojo/issues/12))
* New guide, *Connect Dependency-Track to DefectDojo*, with an example that sets up both sides with this provider and the Dependency-Track provider, for Dependency-Track v5 and v4 ([#14](https://github.com/ottramst/terraform-provider-defectdojo/issues/14))
* Completes the v0.2.0 milestone (SLA configurations, users, notifications and notification webhooks), built on go-defectdojo v0.2.0. Tested against DefectDojo 3.4.304 and 3.5.103 with Terraform 1.16 and 1.17

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
