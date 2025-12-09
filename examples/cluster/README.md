# Cluster Resource Example

This example demonstrates how to create a SleakOps EKS cluster using the Terraform provider.

## Prerequisites

1. SleakOps Core API running at `http://localhost:8000/`
2. Valid credentials (`email` and `password`)
3. Valid `account` ID from your SleakOps Core instance

## Usage

1. Update `main.tf` with your credentials:
   ```hcl
   provider "sleakops" {
     host     = "http://localhost:8000/"
     email    = "admin@sleakops.com"
     password = "admin"
     account  = "504e33c4-5fd4-4dda-a11d-ea526d0e789d"
   }
   ```

2. Initialize Terraform:
   ```bash
   terraform init
   ```

3. Plan the changes:
   ```bash
   terraform plan
   ```

4. Apply the configuration:
   ```bash
   terraform apply
   ```

   Note: Terraform will return when the cluster is in `creating` state. The actual provisioning continues asynchronously.

5. Verify the cluster operation started:
   - Check the Terraform outputs for cluster ID and state (should be `creating`)
   - Monitor in SleakOps Core UI or API until state becomes `created`

6. Destroy the cluster when done:
   ```bash
   terraform destroy
   ```

## Resource Configuration

### Required Fields
- `name`: Cluster name (lowercase alphanumeric with hyphens, max 30 chars)
- `arch`: Architecture - either `"arm64"` or `"amd64"`
- `config.high_availability`: Boolean for HA mode

### Optional Fields
- `description`: Cluster description (max 2500 chars)
- `config.max_memory`: Maximum memory in GB (minimum: 32, default: 256)
- `config.max_cpu`: Maximum CPU cores (minimum: 16, default: 64)

## Notes

- The cluster creation is **asynchronous**. Terraform will return once the cluster reaches `creating` state (not `created`). The actual cluster provisioning continues in the background.
- After `terraform apply`, the cluster state will be `creating`, `updating`, or `deleting` depending on the operation.
- Terraform waits up to 5 minutes for the operation to start, not for it to complete.
- Changing `name` or `arch` requires cluster replacement (destroy + create).
- The provider automatically creates 5 nodepools based on the specified architecture.
- Authentication uses cookie-based sessions with automatic token refresh.

## Troubleshooting

If you encounter errors:

1. **Authentication failures**: Verify credentials and that SleakOps Core API is running
2. **Account errors**: Ensure the `account` UUID exists and you have access
3. **Timeout errors**: Check SleakOps Core logs for cluster creation issues
4. **State mismatch**: Use `terraform refresh` to sync state with actual infrastructure
