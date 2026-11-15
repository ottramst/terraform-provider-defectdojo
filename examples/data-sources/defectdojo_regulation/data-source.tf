data "defectdojo_regulation" "pci" {
  acronym = "PCI DSS"
}

data "defectdojo_regulation" "hipaa" {
  name = "Health Insurance Portability and Accountability Act"
}

data "defectdojo_regulation" "by_id" {
  id = 27
}
