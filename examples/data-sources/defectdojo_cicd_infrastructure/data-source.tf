data "defectdojo_cicd_infrastructure" "jenkins" {
  name                = "Jenkins"
  infrastructure_type = "build_server"
}

data "defectdojo_cicd_infrastructure" "by_id" {
  id = 3
}
