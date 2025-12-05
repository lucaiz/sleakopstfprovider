package provider

import (
	"context"
	"os"

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
	Email    types.String `tfsdk:"email"`
	Password types.String `tfsdk:"password"`
	Account  types.String `tfsdk:"account"`
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
	resp.Schema = schema.Schema{
		Description: "Interact with SleakOps Core API to manage Kubernetes infrastructure, applications, and deployments.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Description: "SleakOps Core API host URL. May also be provided via SLEAKOPS_HOST environment variable.",
				Optional:    true,
			},
			"email": schema.StringAttribute{
				Description: "Email for SleakOps Core authentication. May also be provided via SLEAKOPS_EMAIL environment variable.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Password for SleakOps Core authentication. May also be provided via SLEAKOPS_PASSWORD environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"account": schema.StringAttribute{
				Description: "Account ID for multi-tenant operations. May also be provided via SLEAKOPS_ACCOUNT environment variable.",
				Optional:    true,
			},
		},
	}
}

func (p *sleakopsProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring SleakOps client")

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
			"Unknown SleakOps API Host",
			"The provider cannot create the SleakOps API client as there is an unknown configuration value for the SleakOps API host. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the SLEAKOPS_HOST environment variable.",
		)
	}

	if config.Email.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("email"),
			"Unknown SleakOps API Email",
			"The provider cannot create the SleakOps API client as there is an unknown configuration value for the SleakOps API email. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the SLEAKOPS_EMAIL environment variable.",
		)
	}

	if config.Password.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Unknown SleakOps API Password",
			"The provider cannot create the SleakOps API client as there is an unknown configuration value for the SleakOps API password. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the SLEAKOPS_PASSWORD environment variable.",
		)
	}

	if config.Account.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("account"),
			"Unknown SleakOps Account ID",
			"The provider cannot create the SleakOps API client as there is an unknown configuration value for the SleakOps Account ID. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the SLEAKOPS_ACCOUNT environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	host := os.Getenv("SLEAKOPS_HOST")
	email := os.Getenv("SLEAKOPS_EMAIL")
	password := os.Getenv("SLEAKOPS_PASSWORD")
	account := os.Getenv("SLEAKOPS_ACCOUNT")

	if !config.Host.IsNull() {
		host = config.Host.ValueString()
	}

	if !config.Email.IsNull() {
		email = config.Email.ValueString()
	}

	if !config.Password.IsNull() {
		password = config.Password.ValueString()
	}

	if !config.Account.IsNull() {
		account = config.Account.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.

	if host == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("host"),
			"Missing SleakOps API Host",
			"The provider cannot create the SleakOps API client as there is a missing or empty value for the SleakOps API host. "+
				"Set the host value in the configuration or use the SLEAKOPS_HOST environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if email == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("email"),
			"Missing SleakOps API Email",
			"The provider cannot create the SleakOps API client as there is a missing or empty value for the SleakOps API email. "+
				"Set the email value in the configuration or use the SLEAKOPS_EMAIL environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if password == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Missing SleakOps API Password",
			"The provider cannot create the SleakOps API client as there is a missing or empty value for the SleakOps API password. "+
				"Set the password value in the configuration or use the SLEAKOPS_PASSWORD environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if account == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("account"),
			"Missing SleakOps Account ID",
			"The provider cannot create the SleakOps API client as there is a missing or empty value for the SleakOps Account ID. "+
				"Set the account value in the configuration or use the SLEAKOPS_ACCOUNT environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Validate and parse base URL
	baseURL, err := ParseBaseURL(host)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("host"),
			"Invalid SleakOps API Host",
			"The provider cannot parse the SleakOps API host URL. "+
				"Ensure the host includes a scheme (http:// or https://).\n\n"+
				"Parse Error: "+err.Error(),
		)
		return
	}

	ctx = tflog.SetField(ctx, "sleakops_host", baseURL)
	ctx = tflog.SetField(ctx, "sleakops_email", email)
	ctx = tflog.SetField(ctx, "sleakops_account", account)
	ctx = tflog.SetField(ctx, "sleakops_password", password)
	ctx = tflog.MaskFieldValuesWithFieldKeys(ctx, "sleakops_password")

	tflog.Debug(ctx, "Creating SleakOps client")

	// Create a new SleakOps client using the configuration values
	client, err := NewClient(baseURL, email, password, account)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create SleakOps API Client",
			"An unexpected error occurred when creating the SleakOps API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"SleakOps Client Error: "+err.Error(),
		)
		return
	}

	// Make the SleakOps client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client

	tflog.Info(ctx, "Configured SleakOps client", map[string]any{"success": true})
}

// DataSources defines the data sources implemented in the provider.
func (p *sleakopsProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

// Resources defines the resources implemented in the provider.
func (p *sleakopsProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewClusterResource,
	}
}
