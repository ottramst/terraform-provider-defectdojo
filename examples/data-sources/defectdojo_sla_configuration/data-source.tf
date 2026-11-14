data "defectdojo_sla_configuration" "default" {
  name = "Default"
}

data "defectdojo_sla_configuration" "by_id" {
  id = 3
}
