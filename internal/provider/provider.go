package provider

import (
	"context"
	"os"

	"github.com/hashicorp-demoapp/sleakops-client-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ provider.Provider = &sleakopsProvider{}
)

// New is a helper function to simplify provider server and testing implementation.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &sleakopsProvider{
			version: version,
		}
	}
}

// sleakopsProviderModel maps provider schema data to a Go type.
type sleakopsProviderModel struct {
	Host     types.String `tfsdk:"host"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

// sleakopsProvider is the provider implementation.
type sleakopsProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// Metadata returns the provider type name.
func (p *sleakopsProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "sleakops"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *sleakopsProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{}
}

func (p *sleakopsProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Sleakops client")

	// Retrieve provider data from configuration
	var config sleakopsProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If practitioner provided a configuration value for any of the
	// attributes, it must be a known value.

	if config.Host.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("host"),
			"Unknown Sleakops API Host",
			"The provider cannot create the Sleakops API client as there is an unknown configuration value for the Sleakops API host. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the SLEAKOPS_HOST environment variable.",
		)
	}

	if config.Username.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Unknown Sleakops API Username",
			"The provider cannot create the Sleakops API client as there is an unknown configuration value for the Sleakops API username. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the SLEAKOPS_USERNAME environment variable.",
		)
	}

	if config.Password.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Unknown Sleakops API Password",
			"The provider cannot create the Sleakops API client as there is an unknown configuration value for the Sleakops API password. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the SLEAKOPS_PASSWORD environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	host := os.Getenv("SLEAKOPS_HOST")
	username := os.Getenv("SLEAKOPS_USERNAME")
	password := os.Getenv("SLEAKOPS_PASSWORD")

	if !config.Host.IsNull() {
		host = config.Host.ValueString()
	}

	if !config.Username.IsNull() {
		username = config.Username.ValueString()
	}

	if !config.Password.IsNull() {
		password = config.Password.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.

	if host == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("host"),
			"Missing Sleakops API Host",
			"The provider cannot create the Sleakops API client as there is a missing or empty value for the Sleakops API host. "+
				"Set the host value in the configuration or use the SLEAKOPS_HOST environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if username == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Missing Sleakops API Username",
			"The provider cannot create the Sleakops API client as there is a missing or empty value for the Sleakops API username. "+
				"Set the username value in the configuration or use the SLEAKOPS_USERNAME environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if password == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Missing Sleakops API Password",
			"The provider cannot create the Sleakops API client as there is a missing or empty value for the Sleakops API password. "+
				"Set the password value in the configuration or use the SLEAKOPS_PASSWORD environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "sleakops_host", host)
	ctx = tflog.SetField(ctx, "sleakops_username", username)
	ctx = tflog.SetField(ctx, "sleakops_password", password)
	ctx = tflog.MaskFieldValuesWithFieldKeys(ctx, "sleakops_password")

	tflog.Debug(ctx, "Creating Sleakops client")

	// Create a new Sleakops client using the configuration values
	client, err := sleakops.NewClient(&host, &username, &password)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Sleakops API Client",
			"An unexpected error occurred when creating the Sleakops API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Sleakops Client Error: "+err.Error(),
		)
		return
	}

	// Make the Sleakops client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client

	tflog.Info(ctx, "Configured Sleakops client", map[string]any{"success": true})
}

// DataSources defines the data sources implemented in the provider.
func (p *sleakopsProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

// Resources defines the resources implemented in the provider.
func (p *sleakopsProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}
