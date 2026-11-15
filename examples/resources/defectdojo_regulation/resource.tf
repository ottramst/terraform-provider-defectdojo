resource "defectdojo_regulation" "nis2" {
  name         = "Network and Information Security Directive 2"
  acronym      = "NIS2"
  category     = "security"
  jurisdiction = "European Union"
  description  = "Cybersecurity obligations for essential and important entities."
  reference    = "https://eur-lex.europa.eu/eli/dir/2022/2555/oj"
}

# Products list regulations by ID: a managed one, and one DefectDojo comes with.
data "defectdojo_regulation" "pci" {
  acronym = "PCI DSS"
}

resource "defectdojo_product" "shop" {
  name            = "Web Shop"
  description     = "The customer-facing web shop"
  product_type_id = 4
  regulation_ids  = [defectdojo_regulation.nis2.id, data.defectdojo_regulation.pci.id]
}
