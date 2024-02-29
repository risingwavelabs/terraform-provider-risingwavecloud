// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/fake"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/utils/defaults"
)

const (
	DefaultEndpoint = "https://canary-useast2-acc.risingwave.cloud/api/v1"
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
				Optional:            true,
				Sensitive:           true,
			},
			"api_secret": schema.StringAttribute{
				MarkdownDescription: "The API secret of the your RisingWave Cloud account.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

type RisingWaveCloudProviderModel struct {
	APIKey    types.String `tfsdk:"api_key"`
	APISecret types.String `tfsdk:"api_secret"`
	Endpoint  types.String `tfsdk:"endpoint"`
}

func (p *RisingWaveCloudProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data RisingWaveCloudProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var (
		apiKey    = defaults.String(data.APIKey.ValueString(), os.Getenv("RWC_API_KEY"))
		apiSecret = defaults.String(data.APIKey.ValueString(), os.Getenv("RWC_API_SECRET"))
		endpoint  = defaults.String(data.Endpoint.ValueString(), os.Getenv("RWC_ENDPOINT"))
	)
	if len(endpoint) == 0 {
		endpoint = DefaultEndpoint
	} else { // user specifies their own endpoint
		resp.Diagnostics.AddWarning(
			"API endpoint is provided",
			fmt.Sprintf("Endpoint is only for internal testing. Current endpoint: %s", endpoint),
		)
	}

	if len(apiKey) == 0 {
		resp.Diagnostics.AddError(
			"Missing API Key",
			"RisingWave Cloud API Key is required to setup the provider. "+
				"This can be set either in the provider configuration or in the environment variable RWC_API_KEY. "+
				"Please get your API Key in https://cloud.risingwave.com/",
		)
		return
	}

	if len(apiSecret) == 0 {
		resp.Diagnostics.AddError(
			"Missing API Secret",
			"RisingWave Cloud API Secret is required to setup the provider. "+
				"This can be set either in the provider configuration or in the environment variable RWC_API_SECRET. "+
				"Please get your API Secret in https://cloud.risingwave.com/",
		)
		return
	}

	tflog.Info(ctx, fmt.Sprintf("endpoint: %s", endpoint))

	var client cloudsdk.AccountServiceClientInterface

	if len(os.Getenv("RWC_MOCK")) == 0 {
		acc, err := cloudsdk.NewAccountServiceClient(ctx, endpoint, apiKey, apiSecret)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unexpected error",
				"Failed to build cloud SDK client: "+err.Error(),
			)
			return
		}
		if err := acc.Ping(ctx); err != nil {
			if errors.Is(err, cloudsdk.ErrInvalidCredential) {
				resp.Diagnostics.AddError(
					"Invalid credentials",
					"Please check your API key or API secret",
				)
				return
			}
			resp.Diagnostics.AddError(
				"Failed to connect to the endpoint",
				"Please check your network connection or the endpoint provided",
			)
			return
		}
		client = acc
	} else {
		client = fake.NewFakeAccountServiceClient()
	}

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
