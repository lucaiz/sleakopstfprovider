terraform {
  required_providers {
    sleakops = {
      source = "hashicorp.com/edu/sleakops"
    }
  }
}

provider "sleakops" {
  host     = "http://localhost:19090"
  username = "education"
  password = "test123"
}

data "sleakops_coffees" "edu" {}

output "edu_coffees" {
  value = data.sleakops_coffees.edu
}
