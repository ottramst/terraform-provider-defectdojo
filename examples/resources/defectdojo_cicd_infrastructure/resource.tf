resource "defectdojo_cicd_infrastructure" "github" {
  name                = "GitHub"
  infrastructure_type = "scm_server"
  url                 = "https://github.com"
}

resource "defectdojo_cicd_infrastructure" "jenkins" {
  name                = "Jenkins"
  infrastructure_type = "build_server"
  description         = "The main build server."
  url                 = "https://jenkins.example.com"
}

resource "defectdojo_cicd_infrastructure" "kubernetes" {
  name                = "Production cluster"
  infrastructure_type = "orchestration"
}

# An engagement links to one object of each type.
resource "defectdojo_engagement" "pipeline" {
  product_id      = 7
  name            = "CI pipeline"
  engagement_type = "CI/CD"
  target_start    = "2026-01-01"
  target_end      = "2026-12-31"

  cicd_scm_server_id           = defectdojo_cicd_infrastructure.github.id
  cicd_build_server_id         = defectdojo_cicd_infrastructure.jenkins.id
  cicd_orchestration_engine_id = defectdojo_cicd_infrastructure.kubernetes.id
}
