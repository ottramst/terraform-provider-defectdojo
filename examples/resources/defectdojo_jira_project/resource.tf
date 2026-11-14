# The Jira project of a product: its findings are pushed to the SEC project.
resource "defectdojo_jira_project" "example" {
  jira_instance_id = defectdojo_jira_instance.example.id
  product_id       = defectdojo_product.example.id

  project_key     = "SEC"
  component       = "backend"
  jira_labels     = "security defectdojo"
  push_all_issues = true
  push_notes      = true

  custom_fields = jsonencode({
    customfield_10122 = [{ name = "8.0.1" }]
  })
  close_transition_fields = jsonencode({
    resolution = { name = "Won't Fix" }
  })
}
