// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk"
)

const (
	ProviderSchemaAttrAPIKey   = "api_key"
	ProviderSchemaAttrEndpoint = "endpoint"

	defaultEndpoint = "https://canary-useast2-acc.risingwave.cloud/api/v1"
)

// Assert the provider satisfies various provider interfaces.
var _ provider.Provider = &RisingWaveCloudProvider{}

type RisingWaveCloudProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

func (p *RisingWaveCloudProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "risingwavecloud"
	resp.Version = p.version
}

func (p *RisingWaveCloudProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The endpoint of the RisingWave Cloud API server. This is only used for testing.",
				Optional:            true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "The API key of the your RisingWave Cloud account.",
				Optional:            false,
				Required:            true,
				Sensitive:           true,
			},
		},
	}
}

type RisingWaveCloudProviderModel struct {
	APIKey   types.String `tfsdk:"api_key"`
	Endpoint types.String `tfsdk:"endpoint"`
}

func (p *RisingWaveCloudProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data RisingWaveCloudProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var (
		apiKey   = data.Endpoint.ValueString()
		endpoint = data.Endpoint.ValueString()
	)
	if len(endpoint) == 0 {
		endpoint = defaultEndpoint
	} else { // user specifies their own endpoint
		resp.Diagnostics.AddWarning(
			"API endpoint is provided",
			"Endpoint is only for internal testing.",
		)
	}

	if len(apiKey) == 0 {
		resp.Diagnostics.AddError(
			"Missing API Key",
			"RisingWave Cloud API Key is required to setup the provider. "+
				"Please get your API Key in https://cloud.risingwave.com/",
		)
	}

	client := cloudsdk.NewCloudClient(endpoint, apiKey)
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *RisingWaveCloudProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewClusterResource,
	}
}

func (p *RisingWaveCloudProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewComponentTypeDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &RisingWaveCloudProvider{
			version: version,
		}
	}
}
