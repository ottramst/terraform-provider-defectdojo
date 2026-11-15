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
	"github.com/ottramst/go-defectdojo"
)

// Ensure DefectDojoProvider satisfies various provider interfaces.
var _ provider.Provider = &DefectDojoProvider{}

// DefectDojoProvider defines the provider implementation.
type DefectDojoProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// DefectDojoProviderModel describes the provider data model.
type DefectDojoProviderModel struct {
	URL      types.String `tfsdk:"url"`
	APIToken types.String `tfsdk:"api_token"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

// providerAttribute is a string attribute of the provider configuration with
// the environment variable it falls back to.
type providerAttribute struct {
	name  string
	label string
	env   string
	value types.String
}

func (p *DefectDojoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "defectdojo"
	resp.Version = p.version
}

func (p *DefectDojoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for [OWASP DefectDojo](https://github.com/DefectDojo/django-DefectDojo), " +
			"the open-source vulnerability management platform. " +
			"It supports and is tested against the latest patch of the two newest DefectDojo minor releases, currently 3.4.x and 3.5.x; " +
			"older versions get best-effort support only, and DefectDojo 2.x is not supported.\n\n" +
			"The provider authenticates with an API v2 token (`api_token`), or with a `username` and `password` (HTTP basic authentication), " +
			"which also works when API tokens are disabled on the server. When both are set, the token is used. " +
			"Most configuration objects can only be managed by a superuser.\n\n" +
			"Every attribute can also be set with an environment variable.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				MarkdownDescription: "The URL of the DefectDojo server, such as `https://defectdojo.example.com`. Can also be set with the `DEFECTDOJO_URL` environment variable.",
				Optional:            true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "The API v2 token of the user to authenticate as. Can also be set with the `DEFECTDOJO_API_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "The username for HTTP basic authentication, used when no `api_token` is set. Can also be set with the `DEFECTDOJO_USERNAME` environment variable.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "The password for HTTP basic authentication. Can also be set with the `DEFECTDOJO_PASSWORD` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *DefectDojoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data DefectDojoProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	attrs := []*providerAttribute{
		{name: "url", label: "URL", env: "DEFECTDOJO_URL", value: data.URL},
		{name: "api_token", label: "API token", env: "DEFECTDOJO_API_TOKEN", value: data.APIToken},
		{name: "username", label: "username", env: "DEFECTDOJO_USERNAME", value: data.Username},
		{name: "password", label: "password", env: "DEFECTDOJO_PASSWORD", value: data.Password},
	}
	values := make(map[string]string, len(attrs))
	for _, a := range attrs {
		if a.value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root(a.name),
				"Unknown DefectDojo "+a.label,
				"The provider cannot create the DefectDojo API client as there is an unknown configuration value for the DefectDojo "+a.label+". "+
					"Either target apply the source of the value first, set the value statically in the configuration, or use the "+a.env+" environment variable.",
			)
			continue
		}
		// Default values to environment variables, but override with the
		// Terraform configuration value if set.
		values[a.name] = os.Getenv(a.env)
		if !a.value.IsNull() {
			values[a.name] = a.value.ValueString()
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if values["url"] == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("url"),
			"Missing DefectDojo URL",
			"The provider cannot create the DefectDojo API client as there is a missing or empty value for the DefectDojo URL. "+
				"Set the url value in the configuration or use the DEFECTDOJO_URL environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	var auth defectdojo.ClientOption
	switch {
	case values["api_token"] != "":
		auth = defectdojo.WithAPIToken(values["api_token"])
	case values["username"] != "":
		if values["password"] == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("password"),
				"Missing DefectDojo password",
				"The provider needs a password with the username. Set the password value in the configuration or use the DEFECTDOJO_PASSWORD environment variable.",
			)
		}
		auth = defectdojo.WithBasicAuth(values["username"], values["password"])
	default:
		resp.Diagnostics.AddError(
			"Missing DefectDojo credentials",
			"The provider needs an API token (api_token or DEFECTDOJO_API_TOKEN), or a username and password "+
				"(username and password, or DEFECTDOJO_USERNAME and DEFECTDOJO_PASSWORD).",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := defectdojo.NewClient(values["url"], auth, defectdojo.WithUserAgent("terraform-provider-defectdojo/"+p.version))
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create DefectDojo API client",
			"An unexpected error occurred when creating the DefectDojo API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"DefectDojo Client Error: "+err.Error(),
		)
		return
	}

	// Make the DefectDojo client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *DefectDojoProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewEngagementResource,
		NewNotificationWebhookResource,
		NewNotificationsResource,
		NewNoteTypeResource,
		NewProductResource,
		NewProductTypeResource,
		NewRegulationResource,
		NewSLAConfigurationResource,
		NewSystemSettingsResource,
		NewJiraInstanceResource,
		NewJiraProjectResource,
		NewUserResource,
	}
}

func (p *DefectDojoProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewEngagementDataSource,
		NewProductDataSource,
		NewProductTypeDataSource,
		NewRegulationDataSource,
		NewSLAConfigurationDataSource,
		NewSystemSettingsDataSource,
		NewTestTypesDataSource,
		NewJiraInstanceDataSource,
		NewNoteTypeDataSource,
		NewUserDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &DefectDojoProvider{
			version: version,
		}
	}
}
