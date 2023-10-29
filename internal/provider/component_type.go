// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ComponentTypeDataSource{}

func NewComponentTypeDataSource() datasource.DataSource {
	return &ComponentTypeDataSource{}
}

// ExampleDataSource defines the data source implementation.
type ComponentTypeDataSource struct {
	client *http.Client
}

// ExampleDataSourceModel describes the data source data model.
type ComponentTypeDataSourceModel struct {
	VCPU      types.Int64  `tfsdk:"vcpu"`
	MemoryGiB types.Int64  `tfsdk:"memory_gib"`
	Type      types.String `tfsdk:"type"`
}

func (d *ComponentTypeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_component_type"
}

func (d *ComponentTypeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The type of the component of the RisingWave cluster",
		Attributes: map[string]schema.Attribute{
			"vcpu": schema.StringAttribute{
				MarkdownDescription: "The number of the virtual CPU cores",
				Required:            true,
			},
			"memory_gib": schema.StringAttribute{
				MarkdownDescription: "Memory size in GiB",
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "The type (family) of the component type. The field is reserved for future usage",
				Optional:            true,
			},
		},
	}
}

func (d *ComponentTypeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*http.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *ComponentTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ComponentTypeDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := d.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read example, got error: %s", err))
	//     return
	// }

	// For the purposes of this example code, hardcoding a response value to
	// save into the Terraform state.
	data.Type = types.StringValue("")

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
