data "defectdojo_user" "by_username" {
  username = "jdoe"
}

data "defectdojo_user" "by_id" {
  id = 5
}
