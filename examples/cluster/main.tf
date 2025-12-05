terraform {
  required_providers {
    sleakops = {
      source = "hashicorp.com/edu/sleakops"
    }
  }
}

provider "sleakops" {
  host     = "http://localhost:8000"
  email    = "admin@sleakops.com"
  password = "admin"
  account  = "504e33c4-5fd4-4dda-a11d-ea526d0e789d" # Replace with your account ID
}

resource "sleakops_cluster" "example" {
  name        = "terraform-cluster"
  description = "Cluster managed by Terraform"
  arch        = "arm64" # or "amd64"

  config = {
    max_memory         = 256
    max_cpu            = 64
    high_availability  = true
  }
}

output "cluster_id" {
  description = "The ID of the created cluster"
  value       = sleakops_cluster.example.id
}

output "cluster_state" {
  description = "The current state of the cluster"
  value       = sleakops_cluster.example.state
}

output "cluster_account" {
  description = "The account ID the cluster belongs to"
  value       = sleakops_cluster.example.account
}
