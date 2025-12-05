package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &clusterResource{}
	_ resource.ResourceWithConfigure = &clusterResource{}
)

// NewClusterResource is a helper function to simplify the provider implementation.
func NewClusterResource() resource.Resource {
	return &clusterResource{}
}

// clusterResource is the resource implementation.
type clusterResource struct {
	client *Client
}

// clusterResourceModel maps the resource schema data.
// Maps to apps/cluster/models.py Cluster model
type clusterResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Arch        types.String `tfsdk:"arch"`
	State       types.String `tfsdk:"state"`
	Errors      types.String `tfsdk:"errors"`
	Config      types.Object `tfsdk:"config"` // ClusterConfigModel
	Account     types.String `tfsdk:"account"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
	Deleted     types.Bool   `tfsdk:"deleted"`
}

// clusterConfigModel maps cluster config JSONField
type clusterConfigModel struct {
	MaxMemory         types.Int64 `tfsdk:"max_memory"`
	MaxCPU            types.Int64 `tfsdk:"max_cpu"`
	HighAvailability  types.Bool  `tfsdk:"high_availability"`
}

// clusterAPIRequest represents the request payload for creating/updating clusters
type clusterAPIRequest struct {
	Name        string              `json:"name"`
	Arch        string              `json:"arch,omitempty"` // Only for create
	Description string              `json:"description,omitempty"`
	Config      clusterAPIConfigReq `json:"config"`
}

type clusterAPIConfigReq struct {
	MaxMemory        int  `json:"max_memory,omitempty"`
	MaxCPU           int  `json:"max_cpu,omitempty"`
	HighAvailability bool `json:"high_availability"`
}

// clusterAPIResponse represents the API response from SleakOps Core
type clusterAPIResponse struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	State       string              `json:"state"`
	Errors      string              `json:"errors"`
	Config      clusterAPIConfigRes `json:"config"`
	Description string              `json:"description"`
	Account     string              `json:"account"`
	CreatedAt   string              `json:"created_at"`
	UpdatedAt   string              `json:"updated_at"`
	Deleted     bool                `json:"_deleted"`
	// Nodepools, addons, etc. are ignored for now as they're computed
}

type clusterAPIConfigRes struct {
	MaxMemory        int  `json:"max_memory"`
	MaxCPU           int  `json:"max_cpu"`
	HighAvailability bool `json:"high_availability"`
}

// Metadata returns the resource type name.
func (r *clusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

// Schema defines the schema for the resource.
func (r *clusterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a SleakOps EKS cluster with auto-configured nodepools. Note: Cluster operations are async - Terraform returns when the operation starts (creating/updating/deleting state), not when it completes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Cluster UUID",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Cluster name (lowercase alphanumeric with hyphens, max 30 chars)",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "Cluster description (max 2500 chars)",
				Optional:    true,
				Computed:    true,
			},
			"arch": schema.StringAttribute{
				Description: "Cluster architecture: 'arm64' or 'amd64'. Used during creation to configure nodepools.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"config": schema.SingleNestedAttribute{
				Description: "Cluster configuration",
				Required:    true,
				Attributes: map[string]schema.Attribute{
					"max_memory": schema.Int64Attribute{
						Description: "Maximum memory in GB (minimum: 32)",
						Optional:    true,
						Computed:    true,
						Default:     int64default.StaticInt64(256),
					},
					"max_cpu": schema.Int64Attribute{
						Description: "Maximum CPU cores (minimum: 16)",
						Optional:    true,
						Computed:    true,
						Default:     int64default.StaticInt64(64),
					},
					"high_availability": schema.BoolAttribute{
						Description: "Enable high availability mode",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(true),
					},
				},
			},
			"state": schema.StringAttribute{
				Description: "Cluster state (initial, creating, created, updating, deleting, deleted, failed). After apply, state will be 'creating', 'updating', or 'deleting' as operations are async.",
				Computed:    true,
			},
			"errors": schema.StringAttribute{
				Description: "Error messages if cluster is in failed state",
				Computed:    true,
			},
			"account": schema.StringAttribute{
				Description: "Account ID (set from provider Account header)",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Creation timestamp",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Last update timestamp",
				Computed:    true,
			},
			"deleted": schema.BoolAttribute{
				Description: "Soft delete flag",
				Computed:    true,
			},
		},
	}
}

// Create a new cluster resource.
func (r *clusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Parse config
	var config clusterConfigModel
	diags = plan.Config.As(ctx, &config, basetypes.ObjectAsOptions{})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build request payload
	createReq := clusterAPIRequest{
		Name:        plan.Name.ValueString(),
		Arch:        plan.Arch.ValueString(),
		Description: plan.Description.ValueString(),
		Config: clusterAPIConfigReq{
			MaxMemory:        int(config.MaxMemory.ValueInt64()),
			MaxCPU:           int(config.MaxCPU.ValueInt64()),
			HighAvailability: config.HighAvailability.ValueBool(),
		},
	}

	tflog.Debug(ctx, "Creating cluster", map[string]any{
		"name": createReq.Name,
		"arch": createReq.Arch,
	})

	// Make API request
	httpResp, err := r.client.Post(ctx, "/api/cluster/", createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Cluster",
			"Could not create cluster, unexpected error: "+err.Error(),
		)
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(httpResp.Body)
		resp.Diagnostics.AddError(
			"Error Creating Cluster",
			fmt.Sprintf("API returned status %d: %s", httpResp.StatusCode, string(body)),
		)
		return
	}

	// Parse response
	var clusterResp clusterAPIResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&clusterResp); err != nil {
		resp.Diagnostics.AddError(
			"Error Parsing Cluster Response",
			"Could not parse cluster creation response: "+err.Error(),
		)
		return
	}

	tflog.Info(ctx, "Cluster creation initiated", map[string]any{
		"id":    clusterResp.ID,
		"name":  clusterResp.Name,
		"state": clusterResp.State,
	})

	// Wait for cluster to reach 'creating' state (transitional state is sufficient)
	stableCluster, err := r.waitForClusterState(ctx, clusterResp.ID, []string{"creating", "created", "failed"}, 5*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Waiting for Cluster",
			"Cluster creation timed out or failed: "+err.Error(),
		)
		return
	}

	if stableCluster.State == "failed" {
		resp.Diagnostics.AddError(
			"Cluster Creation Failed",
			fmt.Sprintf("Cluster reached failed state. Errors: %s", stableCluster.Errors),
		)
		return
	}

	tflog.Info(ctx, "Cluster is being created", map[string]any{
		"id":    stableCluster.ID,
		"state": stableCluster.State,
	})

	// Map response to state
	plan.ID = types.StringValue(stableCluster.ID)
	plan.Name = types.StringValue(stableCluster.Name)
	plan.Description = types.StringValue(stableCluster.Description)
	plan.State = types.StringValue(stableCluster.State)
	plan.Errors = types.StringValue(stableCluster.Errors)
	plan.Account = types.StringValue(stableCluster.Account)
	plan.CreatedAt = types.StringValue(stableCluster.CreatedAt)
	plan.UpdatedAt = types.StringValue(stableCluster.UpdatedAt)
	plan.Deleted = types.BoolValue(stableCluster.Deleted)

	// Update config with actual values
	config.MaxMemory = types.Int64Value(int64(stableCluster.Config.MaxMemory))
	config.MaxCPU = types.Int64Value(int64(stableCluster.Config.MaxCPU))
	config.HighAvailability = types.BoolValue(stableCluster.Config.HighAvailability)

	configObj, diags := types.ObjectValueFrom(ctx, plan.Config.AttributeTypes(ctx), config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Config = configObj

	// Set state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ID.ValueString()

	tflog.Debug(ctx, "Reading cluster", map[string]any{"id": clusterID})

	// Get cluster from API
	httpResp, err := r.client.Get(ctx, fmt.Sprintf("/api/cluster/%s/", clusterID))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Cluster",
			"Could not read cluster ID "+clusterID+": "+err.Error(),
		)
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusNotFound {
		// Cluster was deleted outside Terraform
		resp.State.RemoveResource(ctx)
		return
	}

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		resp.Diagnostics.AddError(
			"Error Reading Cluster",
			fmt.Sprintf("API returned status %d: %s", httpResp.StatusCode, string(body)),
		)
		return
	}

	// Parse response
	var clusterResp clusterAPIResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&clusterResp); err != nil {
		resp.Diagnostics.AddError(
			"Error Parsing Cluster Response",
			"Could not parse cluster read response: "+err.Error(),
		)
		return
	}

	// Check soft delete
	if clusterResp.Deleted {
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state
	state.Name = types.StringValue(clusterResp.Name)
	state.Description = types.StringValue(clusterResp.Description)
	state.State = types.StringValue(clusterResp.State)
	state.Errors = types.StringValue(clusterResp.Errors)
	state.Account = types.StringValue(clusterResp.Account)
	state.CreatedAt = types.StringValue(clusterResp.CreatedAt)
	state.UpdatedAt = types.StringValue(clusterResp.UpdatedAt)
	state.Deleted = types.BoolValue(clusterResp.Deleted)

	// Update config
	var config clusterConfigModel
	diags = state.Config.As(ctx, &config, basetypes.ObjectAsOptions{})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.MaxMemory = types.Int64Value(int64(clusterResp.Config.MaxMemory))
	config.MaxCPU = types.Int64Value(int64(clusterResp.Config.MaxCPU))
	config.HighAvailability = types.BoolValue(clusterResp.Config.HighAvailability)

	configObj, diags := types.ObjectValueFrom(ctx, state.Config.AttributeTypes(ctx), config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Config = configObj

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the cluster resource.
func (r *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ID.ValueString()

	// Parse config
	var config clusterConfigModel
	diags = plan.Config.As(ctx, &config, basetypes.ObjectAsOptions{})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build update payload (arch not included in updates)
	updateReq := clusterAPIRequest{
		Description: plan.Description.ValueString(),
		Config: clusterAPIConfigReq{
			HighAvailability: config.HighAvailability.ValueBool(),
		},
	}

	tflog.Debug(ctx, "Updating cluster", map[string]any{"id": clusterID})

	httpResp, err := r.client.Patch(ctx, fmt.Sprintf("/api/cluster/%s/", clusterID), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Cluster",
			"Could not update cluster ID "+clusterID+": "+err.Error(),
		)
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		resp.Diagnostics.AddError(
			"Error Updating Cluster",
			fmt.Sprintf("API returned status %d: %s", httpResp.StatusCode, string(body)),
		)
		return
	}

	// Parse response
	var clusterResp clusterAPIResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&clusterResp); err != nil {
		resp.Diagnostics.AddError(
			"Error Parsing Cluster Response",
			"Could not parse cluster update response: "+err.Error(),
		)
		return
	}

	// Wait for cluster to reach 'updating' state (transitional state is sufficient)
	stableCluster, err := r.waitForClusterState(ctx, clusterID, []string{"updating", "created", "failed"}, 5*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Waiting for Cluster Update",
			"Cluster update timed out or failed: "+err.Error(),
		)
		return
	}

	if stableCluster.State == "failed" {
		resp.Diagnostics.AddError(
			"Cluster Update Failed",
			fmt.Sprintf("Cluster reached failed state. Errors: %s", stableCluster.Errors),
		)
		return
	}

	tflog.Info(ctx, "Cluster is being updated", map[string]any{
		"id":    clusterID,
		"state": stableCluster.State,
	})

	// Update state
	plan.State = types.StringValue(stableCluster.State)
	plan.Errors = types.StringValue(stableCluster.Errors)
	plan.UpdatedAt = types.StringValue(stableCluster.UpdatedAt)

	// Update config with actual values
	config.MaxMemory = types.Int64Value(int64(stableCluster.Config.MaxMemory))
	config.MaxCPU = types.Int64Value(int64(stableCluster.Config.MaxCPU))
	config.HighAvailability = types.BoolValue(stableCluster.Config.HighAvailability)

	configObj, diags := types.ObjectValueFrom(ctx, plan.Config.AttributeTypes(ctx), config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Config = configObj

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the cluster resource.
func (r *clusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ID.ValueString()

	tflog.Debug(ctx, "Deleting cluster", map[string]any{"id": clusterID})

	httpResp, err := r.client.Delete(ctx, fmt.Sprintf("/api/cluster/%s/", clusterID))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Cluster",
			"Could not delete cluster ID "+clusterID+": "+err.Error(),
		)
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusNoContent && httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		resp.Diagnostics.AddError(
			"Error Deleting Cluster",
			fmt.Sprintf("API returned status %d: %s", httpResp.StatusCode, string(body)),
		)
		return
	}

	// Wait for cluster to reach 'deleting' state (transitional state is sufficient)
	deletingCluster, err := r.waitForClusterState(ctx, clusterID, []string{"deleting", "deleted"}, 5*time.Minute)
	if err != nil {
		// Deletion might have succeeded even if we couldn't verify
		tflog.Warn(ctx, "Could not verify cluster deletion state", map[string]any{"error": err.Error()})
	} else {
		tflog.Info(ctx, "Cluster is being deleted", map[string]any{
			"id":    clusterID,
			"state": deletingCluster.State,
		})
	}
}

// Configure adds the provider configured client to the resource.
func (r *clusterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

// waitForClusterState polls the cluster until it reaches one of the target states
func (r *clusterResource) waitForClusterState(ctx context.Context, clusterID string, targetStates []string, timeout time.Duration) (*clusterAPIResponse, error) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeoutChan := time.After(timeout)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutChan:
			return nil, fmt.Errorf("timeout waiting for cluster to reach state %v", targetStates)
		case <-ticker.C:
			httpResp, err := r.client.Get(ctx, fmt.Sprintf("/api/cluster/%s/", clusterID))
			if err != nil {
				tflog.Warn(ctx, "Error polling cluster state", map[string]any{"error": err.Error()})
				continue
			}

			if httpResp.StatusCode != http.StatusOK {
				httpResp.Body.Close()
				tflog.Warn(ctx, "Non-200 status polling cluster", map[string]any{"status": httpResp.StatusCode})
				continue
			}

			var cluster clusterAPIResponse
			if err := json.NewDecoder(httpResp.Body).Decode(&cluster); err != nil {
				httpResp.Body.Close()
				tflog.Warn(ctx, "Error parsing cluster response", map[string]any{"error": err.Error()})
				continue
			}
			httpResp.Body.Close()

			tflog.Debug(ctx, "Cluster state poll", map[string]any{
				"id":    clusterID,
				"state": cluster.State,
			})

			// Check if current state matches any target state
			for _, targetState := range targetStates {
				if cluster.State == targetState {
					return &cluster, nil
				}
			}
		}
	}
}
