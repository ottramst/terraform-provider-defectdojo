## v0.0.1

FEATURES:

* **New Resource:** `defectdojo_product_type`, with resource identity and `force_destroy`, which guards against deleting the products of a product type ([#4](https://github.com/ottramst/terraform-provider-defectdojo/issues/4))
* **New Data Source:** `defectdojo_product_type`, looked up by ID or name ([#4](https://github.com/ottramst/terraform-provider-defectdojo/issues/4))
* provider: authentication with an API v2 token, or a username and password ([#3](https://github.com/ottramst/terraform-provider-defectdojo/issues/3))

NOTES:

* First preview release, built on go-defectdojo v0.0.1. Products and engagements, the rest of the v0.1.0 milestone, follow
* Tested against DefectDojo 3.4.304 and 3.5.103 with Terraform 1.16 and 1.17
