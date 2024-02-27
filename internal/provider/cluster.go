// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/apigen/mgmt"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/utils/wait"
)

// Assert provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClusterResource{}
var _ resource.ResourceWithImportState = &ClusterResource{}

func NewClusterResource() resource.Resource {
	return &ClusterResource{}
}

type ClusterResource struct {
	client cloudsdk.RegionServiceClientInterface
}

var resourceAttrTypes = map[string]attr.Type{
	"id":      types.StringType,
	"replica": types.Int64Type,
}

type EtcdMetaStoreModel struct {
	Resource   types.Object `tfsdk:"resource"`
	EtcdConfig types.String `tfsdk:"etcd_config"`
}

var etcdMetaStoreAttrTypes = map[string]attr.Type{
	"resource": types.ObjectType{
		AttrTypes: resourceAttrTypes,
	},
	"etcd_config": types.StringType,
}

type ComputeSpecModel struct {
	Resource types.Object `tfsdk:"resource"`
}

var computeAttrTypes = resourceAttrTypes

type CompactorSpecModel struct {
	Resource types.Object `tfsdk:"resource"`
}

var compactorAttrTypes = resourceAttrTypes

type FrontendSpecModel struct {
	Resource types.Object `tfsdk:"resource"`
}

var frontendAttrTypes = resourceAttrTypes

type MetaSpecModel struct {
	Resource      types.Object `tfsdk:"resource"`
	EtcdMetaStore types.Object `tfsdk:"etcd_meta_store"`
}

var metaAttrTypes = map[string]attr.Type{
	"resource": types.ObjectType{
		AttrTypes: resourceAttrTypes,
	},
	"etcd_meta_store": types.ObjectType{
		AttrTypes: etcdMetaStoreAttrTypes,
	},
}

type ClusterSpecModel struct {
	ComputeSpec      types.Object `tfsdk:"compute"`
	CompactorSpec    types.Object `tfsdk:"compactor"`
	FrontendSpec     types.Object `tfsdk:"frontend"`
	MetaSpec         types.Object `tfsdk:"meta"`
	RisingWaveConfig types.String `tfsdk:"risingwave_config"`
}

var clusterSpecAttrTypes = map[string]attr.Type{
	"compute": types.ObjectType{
		AttrTypes: computeAttrTypes,
	},
	"compactor": types.ObjectType{
		AttrTypes: compactorAttrTypes,
	},
	"frontend": types.ObjectType{
		AttrTypes: frontendAttrTypes,
	},
	"meta": types.ObjectType{
		AttrTypes: metaAttrTypes,
	},
	"risingwave_config": types.StringType,
}

type ClusterModel struct {
	ID      types.Int64  `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Version types.String `tfsdk:"version"`
	Spec    types.Object `tfsdk:"spec"`
}

type ResourceModel struct {
	Id      types.String `tfsdk:"id"`
	Replica types.Int64  `tfsdk:"replica"`
}

func (r *ClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *ClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resourceAttribute := schema.SingleNestedAttribute{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The component type ID of the node",
				Required:            true,
			},
			"replica": schema.Int64Attribute{
				MarkdownDescription: "The number of nodes",
				Computed:            true,
				Optional:            true,
				Default:             int64default.StaticInt64(1),
			},
		},
		MarkdownDescription: "The resource specification of the component",
		Required:            true,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "A RisingWave Cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The id of the cluster.",
				Optional:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster.",
				Required:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "The RisingWave cluster version." +
					"It is used to fetch the image from the official image registery of RisingWave Labs." +
					"The newest stable version will be used if this field is not present.",
				Optional: true,
			},
			"spec": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"compute": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"resource": resourceAttribute,
						},
						Required: true,
					},
					"compactor": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"resource": resourceAttribute,
						},
						Required: true,
					},
					"frontend": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"resource": resourceAttribute,
						},
						Required: true,
					},
					"meta": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"resource": resourceAttribute,
							"etcd_meta_store": schema.SingleNestedAttribute{
								Attributes: map[string]schema.Attribute{
									"resource": resourceAttribute,
									"etcd_config": schema.StringAttribute{
										MarkdownDescription: "The environment variable list of the etcd configuration",
										Optional:            true,
										Computed:            true,
										Default:             stringdefault.StaticString(""),
									},
								},
								Optional: true,
							},
						},
						Required: true,
					},
					"risingwave_config": schema.StringAttribute{
						MarkdownDescription: "The toml format of the RisingWave configuration of the cluster",
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString(""),
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

	client, ok := req.ProviderData.(cloudsdk.RegionServiceClientInterface)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected cloudsdk.RegionServiceClientInterface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func clusterToDataModel(cluster *apigen_mgmt.Tenant, data *ClusterModel) {
	data.Name = types.StringValue(cluster.TenantName)
	data.Version = types.StringValue(cluster.ImageTag)
	data.ID = types.Int64Value(int64(cluster.Id))
	data.Spec = types.ObjectValueMust(
		clusterSpecAttrTypes,
		map[string]attr.Value{
			"risingwave_config": types.StringValue(cluster.RwConfig),
			"compute": types.ObjectValueMust(
				computeAttrTypes,
				map[string]attr.Value{
					"resource": types.ObjectValueMust(
						resourceAttrTypes,
						map[string]attr.Value{
							"id":      types.StringValue(cluster.Resources.Components.Compute.ComponentTypeId),
							"replica": types.Int64Value(int64(cluster.Resources.Components.Compute.Replica)),
						},
					),
				},
			),
			"compactor": types.ObjectValueMust(
				compactorAttrTypes,
				map[string]attr.Value{
					"resource": types.ObjectValueMust(
						resourceAttrTypes,
						map[string]attr.Value{
							"id":      types.StringValue(cluster.Resources.Components.Compactor.ComponentTypeId),
							"replica": types.Int64Value(int64(cluster.Resources.Components.Compactor.Replica)),
						},
					),
				},
			),
			"frontend": types.ObjectValueMust(
				frontendAttrTypes,
				map[string]attr.Value{
					"resource": types.ObjectValueMust(
						resourceAttrTypes,
						map[string]attr.Value{
							"id":      types.StringValue(cluster.Resources.Components.Frontend.ComponentTypeId),
							"replica": types.Int64Value(int64(cluster.Resources.Components.Frontend.Replica)),
						},
					),
				},
			),
			"meta": types.ObjectValueMust(
				metaAttrTypes,
				map[string]attr.Value{
					"resource": types.ObjectValueMust(
						resourceAttrTypes,
						map[string]attr.Value{
							"id":      types.StringValue(cluster.Resources.Components.Meta.ComponentTypeId),
							"replica": types.Int64Value(int64(cluster.Resources.Components.Meta.Replica)),
						},
					),
					"etcd_meta_store": types.ObjectValueMust(
						etcdMetaStoreAttrTypes,
						map[string]attr.Value{
							"resource": types.ObjectValueMust(
								resourceAttrTypes,
								map[string]attr.Value{
									"id":      types.StringValue(cluster.Resources.Components.Etcd.ComponentTypeId),
									"replica": types.Int64Value(int64(cluster.Resources.Components.Etcd.Replica)),
								},
							),
							"etcd_config": types.StringValue(cluster.RwConfig),
						},
					),
				},
			),
		},
	)
}

func (r *ClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	objectAsOptions := basetypes.ObjectAsOptions{
		UnhandledUnknownAsEmpty: true,
		UnhandledNullAsEmpty:    true,
	}

	var (
		spec              ClusterSpecModel
		compactorSpec     CompactorSpecModel
		compactorResource ResourceModel
		computeSpec       ComputeSpecModel
		computeResource   ResourceModel
		frontendSpec      FrontendSpecModel
		frontendResource  ResourceModel
		metaSpec          MetaSpecModel
		metaResource      ResourceModel

		useEtcdMetaStore bool
		etcdMetaStore    EtcdMetaStoreModel
		etcdResource     ResourceModel
	)

	resp.Diagnostics.Append(data.Spec.As(ctx, &spec, objectAsOptions)...)

	resp.Diagnostics.Append(spec.CompactorSpec.As(ctx, &compactorSpec, objectAsOptions)...)
	resp.Diagnostics.Append(compactorSpec.Resource.As(ctx, &compactorResource, objectAsOptions)...)

	resp.Diagnostics.Append(spec.ComputeSpec.As(ctx, &computeSpec, objectAsOptions)...)
	resp.Diagnostics.Append(computeSpec.Resource.As(ctx, &computeResource, objectAsOptions)...)

	resp.Diagnostics.Append(spec.FrontendSpec.As(ctx, &frontendSpec, objectAsOptions)...)
	resp.Diagnostics.Append(frontendSpec.Resource.As(ctx, &frontendResource, objectAsOptions)...)

	resp.Diagnostics.Append(spec.MetaSpec.As(ctx, &metaSpec, objectAsOptions)...)
	resp.Diagnostics.Append(metaSpec.Resource.As(ctx, &etcdResource, objectAsOptions)...)

	resp.Diagnostics.Append(spec.MetaSpec.As(ctx, &metaSpec, objectAsOptions)...)
	if !metaSpec.EtcdMetaStore.IsNull() {
		useEtcdMetaStore = true
		resp.Diagnostics.Append(metaSpec.EtcdMetaStore.As(ctx, &etcdMetaStore, objectAsOptions)...)
		resp.Diagnostics.Append(etcdMetaStore.Resource.As(ctx, &etcdResource, objectAsOptions)...)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	if !useEtcdMetaStore {
		resp.Diagnostics.AddError(
			"Missing meta store",
			"Meta store is required to setup the cluster.",
		)
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	var tenantReq = apigen_mgmt.TenantRequestRequestBody{}
	tenantReq.TenantName = data.Name.ValueString()
	tenantReq.ImageTag = data.Version.ValueStringPointer()
	tenantReq.Tier = &DefaultTier
	tenantReq.Resources = &apigen_mgmt.TenantResourceRequest{
		Components: apigen_mgmt.TenantResourceRequestComponents{
			Compute: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: computeResource.Id.ValueString(),
				Replica:         int(computeResource.Replica.ValueInt64()),
			},
			Frontend: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: frontendResource.Id.ValueString(),
				Replica:         int(frontendResource.Replica.ValueInt64()),
			},
			Meta: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: metaResource.Id.ValueString(),
				Replica:         int(metaResource.Replica.ValueInt64()),
			},
			Compactor: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: compactorResource.Id.ValueString(),
				Replica:         int(compactorResource.Replica.ValueInt64()),
			},
		},
		ComputeFileCacheSizeGiB: 20,
		EnableComputeFileCache:  true,
		EtcdVolumeSizeGiB:       20,
	}

	if useEtcdMetaStore {
		tenantReq.Resources.Components.Etcd = apigen_mgmt.ComponentResourceRequest{
			ComponentTypeId: etcdResource.Id.ValueString(),
			Replica:         int(etcdResource.Replica.ValueInt64()),
		}
	}

	cluster, err := r.client.CreateClusterAwait(ctx, tenantReq)
	if err != nil {
		if errors.Is(err, wait.ErrWaitTimeout) {
			resp.Diagnostics.AddError(
				"Timeout while waiting",
				fmt.Sprintf("The cluster did not reach the desired state before the timeout: %s", err.Error()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to create cluster",
			err.Error(),
		)
		return
	}

	clusterToDataModel(cluster, &data)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, fmt.Sprintf("cluster created, id: %d", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterModel

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
	var data ClusterModel

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
	var data ClusterModel

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
