# Find the CI/CD infrastructure objects, and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_cicd_infrastructure" "all" {
  provider = defectdojo
}

# Only build servers whose name contains "jenkins", ignoring case
list "defectdojo_cicd_infrastructure" "jenkins" {
  provider = defectdojo

  config {
    infrastructure_type = "build_server"
    name_contains       = "jenkins"
  }
}
