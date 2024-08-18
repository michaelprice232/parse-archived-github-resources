# parse-archived-github-resources

An opinionated app used to identify GitHub repository Terraform resources (both explicit and module based) which are marked as archived.
Starts from a root directory and then for all child directories one level down, generates a shell script which includes terragrunt
state removal commands for all the archived resources. 

This project was used to facilitate the mass migration of archived GH repo's from one org to another. 
A [separate script](https://github.com/michaelprice232/migrate-archived-github-repos) was used
to perform the actual repo transfer and this script was used to clean up the Terraform state. The actual config removal from git
was out of scope for this script and was performed via manual PR's.

## Types of Terraform resources it looks for

```hcl
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
```

## Example output file (one per source Terraform file)
```shell
terragrunt state rm github_repository.repo_1
terragrunt state rm module.repo_2
```

## How to run

```shell
# --root-dir - path to where we should start processing Terraform file from. Looks in directories one level down only
# --output-dir - path to where we should write the Terragrunt files containing the state removal commands for the archived resources

go run ./main.go --root-dir "./path/to/root" --output-dir "./where/to/output/to"
```