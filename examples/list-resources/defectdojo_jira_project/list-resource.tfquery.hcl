# Find the Jira projects of products and engagements, and generate import
# blocks and configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_jira_project" "all" {
  provider = defectdojo
}

# Only the Jira project of one product
list "defectdojo_jira_project" "shop" {
  provider = defectdojo

  config {
    product_id = 1
  }
}
