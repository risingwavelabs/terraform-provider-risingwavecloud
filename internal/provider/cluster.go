// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk"
)

// Assert provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClusterResource{}
var _ resource.ResourceWithImportState = &ClusterResource{}

func NewClusterResource() resource.Resource {
	return &ClusterResource{}
}

// ExampleResource defines the resource implementation.
type ClusterResource struct {
	client cloudsdk.CloudClientInterface
}

// ExampleResourceModel describes the resource data model.
type ClusterResourceModel struct {
	Name       types.String `tfsdk:"name"`
	Platform   types.String `tfsdk:"platform"`
	Region     types.String `tfsdk:"region"`
	Version    types.String `tfsdk:"version"`
	ResourceV1 types.Object `tfsdk:"resource_v1"`
}

type ResourceV1Model struct {
	Compute                types.Object `tfsdk:"compute"`
	Compactor              types.Object `tfsdk:"compactor"`
	Frontend               types.Object `tfsdk:"frontend"`
	Meta                   types.Object `tfsdk:"meta"`
	Etcd                   types.Object `tfsdk:"etcd"`
	EtcdDiskSizeGB         types.Int64  `tfsdk:"etcd_disk_size_gb"`
	ComputeFileCacheSizeGB types.Int64  `tfsdk:"compute_file_cache_size_gb"`
}

type ComponentModel struct {
	Type    types.String `tfsdk:"type"`
	Replica types.Int64  `tfsdk:"replica"`
}

func (r *ClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *ClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	var componentAttribute = schema.MapNestedAttribute{
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"type": schema.StringAttribute{
					MarkdownDescription: "The component type of the node",
					Required:            true,
				},
				"replica": schema.Int64Attribute{
					MarkdownDescription: "The number of nodes",
					Computed:            true,
					Optional:            true,
					Default:             int64default.StaticInt64(1),
				},
			},
		},
		MarkdownDescription: "The resource specification of the component",
		Required:            true,
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A RisingWave Cluster",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster.",
				Required:            true,
			},
			"platform": schema.StringAttribute{
				MarkdownDescription: "The cloud platform to host this cluster",
				Required:            true,
				Optional:            false,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "The region to host this cluster",
				Required:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "The RisingWave cluster version." +
					"It is used to fetch the image from the official image registery of RisingWave Labs." +
					"The newest stable version will be used if this field is not present.",
				Optional: true,
			},
			"resource_v1": schema.MapNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"compute":   componentAttribute,
						"compactor": componentAttribute,
						"etcd":      componentAttribute,
						"frontend":  componentAttribute,
						"meta":      componentAttribute,
						"etcd_disk_size_gb": schema.Int64Attribute{
							MarkdownDescription: "The disk size of the etcd pod",
							Computed:            true,
							Optional:            true,
							Default:             int64default.StaticInt64(32),
						},
						"compute_file_cache_size_gb": schema.Int64Attribute{
							MarkdownDescription: "The disk size of the compute file cache. 0 means disabling the compute file cache",
							Optional:            true,
							Computed:            true,
							Default:             int64default.StaticInt64(0),
						},
					},
				},
				Required:            true,
				MarkdownDescription: "The resource specification of the cluster",
			},
		},
	}
}

func (r *ClusterResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(cloudsdk.CloudClientInterface)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.CloudClientInterface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

// func unmarshalDataModel(resp *resource.CreateResponse) *ResourceV1Model {

// }

func (r *ClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create example, got error: %s", err))
	//     return
	// }

	// For the purposes of this example code, hardcoding a response value to
	// save into the Terraform state.
	var resourcev1Spec ResourceV1Model
	resp.Diagnostics.Append(data.ResourceV1.As(ctx, &resourcev1Spec, basetypes.ObjectAsOptions{})...)
	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read example, got error: %s", err))
	//     return
	// }

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ClusterResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update example, got error: %s", err))
	//     return
	// }

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClusterResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete example, got error: %s", err))
	//     return
	// }
}

func (r *ClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
