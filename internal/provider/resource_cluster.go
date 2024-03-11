// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

var (
	DefaultEnableComputeFileCache = true
	DefaultComputeFileCacheSizeGB = 20
	DefaultEtcdVolumeSizeGB       = 10
)

// Assert provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClusterResource{}
var _ resource.ResourceWithImportState = &ClusterResource{}

func NewClusterResource() resource.Resource {
	return &ClusterResource{}
}

type ClusterResource struct {
	client cloudsdk.CloudClientInterface
}

var resourceAttrTypes = map[string]attr.Type{
	"id":      types.StringType,
	"replica": types.Int64Type,
}

var componentAttrTypes = map[string]attr.Type{
	"resource": types.ObjectType{
		AttrTypes: resourceAttrTypes,
	},
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

var computeAttrTypes = componentAttrTypes

type CompactorSpecModel struct {
	Resource types.Object `tfsdk:"resource"`
}

var compactorAttrTypes = componentAttrTypes

type FrontendSpecModel struct {
	Resource types.Object `tfsdk:"resource"`
}

var frontendAttrTypes = componentAttrTypes

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
	NsID    types.String `tfsdk:"nsid"`
	Region  types.String `tfsdk:"region"`
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
				Optional:            true,
				Default:             int64default.StaticInt64(1),
				Computed:            true,
			},
		},
		MarkdownDescription: "The resource specification of the component",
		Required:            true,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "A RisingWave Cluster",
		Attributes: map[string]schema.Attribute{
			"nsid": schema.StringAttribute{
				MarkdownDescription: "The namespace id of the cluster.",
				Computed:            true,
			},
			"region": schema.StringAttribute{
				Required: true,
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
										Default:             stringdefault.StaticString(""),
										Computed:            true,
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
						Default:             stringdefault.StaticString(""),
						Computed:            true,
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
			fmt.Sprintf("Expected cloudsdk.AccountServiceClientInterface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func clusterToDataModel(cluster *apigen_mgmt.Tenant, data *ClusterModel) {
	data.Name = types.StringValue(cluster.TenantName)
	data.Version = types.StringValue(cluster.ImageTag)
	data.NsID = types.StringValue(cluster.NsId.String())
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
							"etcd_config": types.StringValue(cluster.EtcdConfig),
						},
					),
				},
			),
		},
	)
}

func dataModelToCluster(ctx context.Context, data *ClusterModel, cluster *apigen_mgmt.Tenant) diag.Diagnostics {
	diags := diag.Diagnostics{}
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

	tflog.Trace(ctx, "parsing spec")
	diags.Append(data.Spec.As(ctx, &spec, objectAsOptions)...)

	tflog.Trace(ctx, "parsing compactorSpec")
	diags.Append(spec.CompactorSpec.As(ctx, &compactorSpec, objectAsOptions)...)
	diags.Append(compactorSpec.Resource.As(ctx, &compactorResource, objectAsOptions)...)

	tflog.Trace(ctx, "parsing computeSpec")
	diags.Append(spec.ComputeSpec.As(ctx, &computeSpec, objectAsOptions)...)
	diags.Append(computeSpec.Resource.As(ctx, &computeResource, objectAsOptions)...)

	tflog.Trace(ctx, "parsing frontendSpec")
	diags.Append(spec.FrontendSpec.As(ctx, &frontendSpec, objectAsOptions)...)
	diags.Append(frontendSpec.Resource.As(ctx, &frontendResource, objectAsOptions)...)

	tflog.Trace(ctx, "parsing metaSpec")
	diags.Append(spec.MetaSpec.As(ctx, &metaSpec, objectAsOptions)...)
	diags.Append(metaSpec.Resource.As(ctx, &metaResource, objectAsOptions)...)

	if !metaSpec.EtcdMetaStore.IsNull() {
		tflog.Trace(ctx, "parsing etcdMetaStore")
		useEtcdMetaStore = true
		diags.Append(metaSpec.EtcdMetaStore.As(ctx, &etcdMetaStore, objectAsOptions)...)
		diags.Append(etcdMetaStore.Resource.As(ctx, &etcdResource, objectAsOptions)...)
	}

	if !useEtcdMetaStore {
		diags.AddError(
			"Missing meta store",
			"Meta store is required to setup the cluster.",
		)
		return diags
	}

	nsId, err := uuid.Parse(data.NsID.String())
	if err != nil {
		diags.AddError(
			"Failed to parse nsid",
			fmt.Sprintf("Cannot parse cluster NsID %s", data.NsID.String()),
		)
		return diags
	}
	cluster.NsId = nsId
	cluster.TenantName = data.Name.ValueString()
	cluster.ImageTag = data.Version.ValueString()
	cluster.Tier = apigen_mgmt.TierId(DefaultTier)
	cluster.RwConfig = spec.RisingWaveConfig.ValueString()
	cluster.EtcdConfig = etcdMetaStore.EtcdConfig.ValueString()
	cluster.Region = data.Region.ValueString()
	cluster.Resources = apigen_mgmt.TenantResource{
		Components: apigen_mgmt.TenantResourceComponents{
			Compactor: &apigen_mgmt.ComponentResource{
				ComponentTypeId: compactorResource.Id.ValueString(),
				Replica:         int(compactorResource.Replica.ValueInt64()),
			},
			Compute: &apigen_mgmt.ComponentResource{
				ComponentTypeId: computeResource.Id.ValueString(),
				Replica:         int(computeResource.Replica.ValueInt64()),
			},
			Frontend: &apigen_mgmt.ComponentResource{
				ComponentTypeId: frontendResource.Id.ValueString(),
				Replica:         int(frontendResource.Replica.ValueInt64()),
			},
			Meta: &apigen_mgmt.ComponentResource{
				ComponentTypeId: metaResource.Id.ValueString(),
				Replica:         int(metaResource.Replica.ValueInt64()),
			},
			Etcd: apigen_mgmt.ComponentResource{
				ComponentTypeId: etcdResource.Id.ValueString(),
				Replica:         int(etcdResource.Replica.ValueInt64()),
			},
		},
		ComputeFileCacheSizeGiB: DefaultComputeFileCacheSizeGB,
		EnableComputeFileCache:  DefaultEnableComputeFileCache,
		EtcdVolumeSizeGiB:       DefaultEtcdVolumeSizeGB,
	}

	return diags
}

func (r *ClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var (
		region = data.Region.String()
	)

	var cluster apigen_mgmt.Tenant

	if len(region) == 0 {
		resp.Diagnostics.AddError(
			"Invalid region",
			"Region is required",
		)
		return
	}

	dataModelToCluster(ctx, &data, &cluster)

	raw, _ := json.Marshal(data)
	fmt.Println(string(raw))

	if resp.Diagnostics.HasError() {
		return
	}

	exist, err := r.client.IsTenantNameExist(ctx, region, cluster.TenantName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to check cluster existence",
			err.Error(),
		)
		return
	}
	if exist {
		resp.Diagnostics.AddError(
			"Cluster name already exists",
			fmt.Sprintf(
				"Cluster with name %s already exists, please use `terraform import` command to manage existing clusters",
				cluster.TenantName,
			),
		)
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	var tenantReq = apigen_mgmt.TenantRequestRequestBody{}
	tenantReq.TenantName = cluster.TenantName
	tenantReq.ImageTag = &cluster.ImageTag
	tenantReq.Tier = &DefaultTier
	tenantReq.RwConfig = &cluster.RwConfig
	tenantReq.EtcdConfig = &cluster.EtcdConfig
	tenantReq.Resources = &apigen_mgmt.TenantResourceRequest{
		Components: apigen_mgmt.TenantResourceRequestComponents{
			Compute: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: cluster.Resources.Components.Compute.ComponentTypeId,
				Replica:         cluster.Resources.Components.Compute.Replica,
			},
			Frontend: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: cluster.Resources.Components.Frontend.ComponentTypeId,
				Replica:         cluster.Resources.Components.Frontend.Replica,
			},
			Meta: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: cluster.Resources.Components.Meta.ComponentTypeId,
				Replica:         cluster.Resources.Components.Meta.Replica,
			},
			Compactor: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: cluster.Resources.Components.Compactor.ComponentTypeId,
				Replica:         cluster.Resources.Components.Compactor.Replica,
			},
			Etcd: apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: cluster.Resources.Components.Etcd.ComponentTypeId,
				Replica:         cluster.Resources.Components.Etcd.Replica,
			},
		},
		ComputeFileCacheSizeGiB: cluster.Resources.ComputeFileCacheSizeGiB,
		EnableComputeFileCache:  cluster.Resources.EnableComputeFileCache,
		EtcdVolumeSizeGiB:       cluster.Resources.EtcdVolumeSizeGiB,
	}

	createdCluster, err := r.client.CreateClusterAwait(ctx, region, tenantReq)
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

	clusterToDataModel(createdCluster, &data)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Info(ctx, fmt.Sprintf("cluster created, UUID: %s", createdCluster.NsId))

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

	// minimal identifiers for import state
	nsID, err := uuid.Parse(data.NsID.String())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to parse nsid",
			fmt.Sprintf("Cannot parse cluster NsID %s", data.NsID.String()),
		)
		return
	}

	cluster, err := r.client.GetClusterByNsID(ctx, nsID)
	if err != nil {
		// Ignore returning errors that signify the resource is no longer existent,
		// call the response state RemoveResource() method, and return early.
		// The next Terraform plan will recreate the resource.
		if errors.Is(err, cloudsdk.ErrClusterNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to read cluster",
			err.Error(),
		)
		return
	}

	clusterToDataModel(cluster, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func resourceEqual(a, b *apigen_mgmt.ComponentResource) bool {
	return a.ComponentTypeId == b.ComponentTypeId && a.Replica == b.Replica
}

func (r *ClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ClusterModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	// minimal identifiers for import state
	nsID, err := uuid.Parse(data.NsID.String())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to parse nsid",
			fmt.Sprintf("Cannot parse cluster NsID %s", data.NsID.String()),
		)
		return
	}

	if resp.Diagnostics.HasError() {
		return
	}

	var updated = apigen_mgmt.Tenant{}

	dataModelToCluster(ctx, &data, &updated)

	previous, err := r.client.GetClusterByNsID(ctx, nsID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read cluster",
			err.Error(),
		)
		return
	}

	// assign ID as the ID is obtained by computing.
	data.NsID = types.StringValue(previous.NsId.String())

	// immutable fields
	if previous.Resources.EnableComputeFileCache != updated.Resources.EnableComputeFileCache {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			"Compute file cache cannot be changed",
		)
	}
	if previous.Resources.ComputeFileCacheSizeGiB != updated.Resources.ComputeFileCacheSizeGiB {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			"Compute file cache size cannot be changed",
		)
	}
	if previous.Resources.EtcdVolumeSizeGiB != updated.Resources.EtcdVolumeSizeGiB {
		resp.Diagnostics.AddError(
			"Cannot update updatete immutable field",
			"Etcd volume size cannot be changed",
		)
	}
	if previous.TenantName != updated.TenantName {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			"Cluster name cannot be changed",
		)
	}

	if !resourceEqual(&previous.Resources.Components.Etcd, &updated.Resources.Components.Etcd) {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			"Etcd resource cannot be changed",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// update version
	if previous.ImageTag != updated.ImageTag {
		tflog.Info(ctx, fmt.Sprintf("updating version from %s to %s, cluster: %s", previous.ImageTag, updated.ImageTag, previous.TenantName))
		if err := r.client.UpdateClusterImageByNsIDAwait(ctx, nsID, updated.ImageTag); err != nil {
			if errors.Is(err, wait.ErrWaitTimeout) {
				resp.Diagnostics.AddError(
					"Timeout while waiting",
					fmt.Sprintf("The cluster did not reach the desired state before the timeout: %s", err.Error()),
				)
				return
			}
			resp.Diagnostics.AddError(
				"Unable to update cluster version",
				err.Error(),
			)
			return
		}
		tflog.Info(ctx, "cluster version updated")
	}

	// update rwconfig
	if previous.RwConfig != updated.RwConfig {
		tflog.Info(ctx, fmt.Sprintf("updating risingwave configuration, cluster: %s", previous.TenantName))
		if err := r.client.UpdateRisingWaveConfigByNsIDAwait(ctx, nsID, updated.RwConfig); err != nil {
			if errors.Is(err, wait.ErrWaitTimeout) {
				resp.Diagnostics.AddError(
					"Timeout while waiting",
					fmt.Sprintf("The cluster did not reach the desired state before the timeout: %s", err.Error()),
				)
				return
			}
			resp.Diagnostics.AddError(
				"Unable to update cluster risingwave config",
				err.Error(),
			)
			return
		}
		tflog.Info(ctx, "cluster risingwave configuration updated")
	}

	// update etcd config
	if previous.EtcdConfig != updated.EtcdConfig {
		tflog.Info(ctx, fmt.Sprintf("updating etcd configuration, cluster: %s", previous.TenantName))
		if err := r.client.UpdateEtcdConfigByNsIDAwait(ctx, nsID, updated.EtcdConfig); err != nil {
			if errors.Is(err, wait.ErrWaitTimeout) {
				resp.Diagnostics.AddError(
					"Timeout while waiting",
					fmt.Sprintf("The cluster did not reach the desired state before the timeout: %s", err.Error()),
				)
				return
			}
			resp.Diagnostics.AddError(
				"Unable to update cluster etcd config",
				err.Error(),
			)
			return
		}
		tflog.Info(ctx, "cluster etcd configuration updated")
	}

	// update cluster components
	if !(resourceEqual(previous.Resources.Components.Compute, updated.Resources.Components.Compute) &&
		resourceEqual(previous.Resources.Components.Compactor, updated.Resources.Components.Compactor) &&
		resourceEqual(previous.Resources.Components.Frontend, updated.Resources.Components.Frontend) &&
		resourceEqual(previous.Resources.Components.Meta, updated.Resources.Components.Meta)) {

		tflog.Info(ctx, fmt.Sprintf("updating resources, cluster: %s", previous.TenantName))
		if err := r.client.UpdateClusterResourcesByNsIDAwait(ctx, nsID, apigen_mgmt.PostTenantResourcesRequestBody{
			Compute: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: updated.Resources.Components.Compute.ComponentTypeId,
				Replica:         updated.Resources.Components.Compute.Replica,
			},
			Compactor: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: updated.Resources.Components.Compactor.ComponentTypeId,
				Replica:         updated.Resources.Components.Compactor.Replica,
			},
			Frontend: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: updated.Resources.Components.Frontend.ComponentTypeId,
				Replica:         updated.Resources.Components.Frontend.Replica,
			},
			Meta: &apigen_mgmt.ComponentResourceRequest{
				ComponentTypeId: updated.Resources.Components.Meta.ComponentTypeId,
				Replica:         updated.Resources.Components.Meta.Replica,
			},
		}); err != nil {
			if errors.Is(err, wait.ErrWaitTimeout) {
				resp.Diagnostics.AddError(
					"Timeout while waiting",
					fmt.Sprintf("The cluster did not reach the desired state before the timeout: %s", err.Error()),
				)
				return
			}
			resp.Diagnostics.AddError(
				"Unable to update cluster resources",
				err.Error(),
			)
			return
		}
		tflog.Info(ctx, "cluster resources updated")
	}

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

	// minimal identifiers for import state
	nsID, err := uuid.Parse(data.NsID.String())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to parse nsid",
			fmt.Sprintf("Cannot parse cluster NsID %s", data.NsID.String()),
		)
		return
	}

	if err := r.client.DeleteClusterByNsIDAwait(ctx, nsID); err != nil {
		if errors.Is(err, wait.ErrWaitTimeout) {
			resp.Diagnostics.AddError(
				"Timeout while waiting",
				fmt.Sprintf("The cluster did not reach the desired state before the timeout: %s", err.Error()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to delete cluster",
			err.Error(),
		)
		return
	}
}

func (r *ClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nsID, err := uuid.Parse(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Failed to parse id: %s", nsID),
		)
		return

	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("nsid"), nsID.String())...)
}
