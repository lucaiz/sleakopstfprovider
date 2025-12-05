terraform {
  required_providers {
    sleakops = {
      source = "hashicorp.com/edu/sleakops"
    }
  }
  required_version = ">= 1.1.0"
}

provider "sleakops" {
  username = "education"
  password = "test123"
  host     = "http://localhost:19090"
}

resource "sleakops_order" "edu" {
  items = [{
    coffee = {
      id = 3
    }
    quantity = 2
    },
    {
      coffee = {
        id = 2
      }
      quantity = 3
  }]
}

output "edu_order" {
  value = sleakops_order.edu
}
