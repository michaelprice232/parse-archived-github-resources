module "example_repo" {
  source       = "github.com/michaelprice232/terraform-module-github-repository?ref=<version>"
  name         = "test-repo"
  archived     = true     # Checks for resources in which this is true
}


resource "github_repository" "another_example_repo" {
  name        = "another-test-repo"
  description = "Some description"
  visibility  = "private"
  archived    = true    # Checks for resources in which this is true
}