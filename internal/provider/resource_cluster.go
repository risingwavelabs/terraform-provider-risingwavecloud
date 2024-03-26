package provider

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

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
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/wait"
)

var (
	DefaultTier                   = apigen_mgmt.Standard
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

var defaultNodeGroup = map[string]attr.Type{
	"cpu":     types.StringType,
	"memory":  types.StringType,
	"replica": types.Int64Type,
}

var componentAttrTypes = map[string]attr.Type{
	"default_node_group": types.ObjectType{
		AttrTypes: defaultNodeGroup,
	},
}

type EtcdMetaStoreModel struct {
	DefaultNodeGroup types.Object `tfsdk:"default_node_group"`
	EtcdConfig       types.String `tfsdk:"etcd_config"`
}

var etcdMetaStoreAttrTypes = map[string]attr.Type{
	"default_node_group": types.ObjectType{
		AttrTypes: defaultNodeGroup,
	},
	"etcd_config": types.StringType,
}

type ComputeSpecModel struct {
	DefaultNodeGroup types.Object `tfsdk:"default_node_group"`
}

var computeAttrTypes = componentAttrTypes

type CompactorSpecModel struct {
	DefaultNodeGroup types.Object `tfsdk:"default_node_group"`
}

var compactorAttrTypes = componentAttrTypes

type FrontendSpecModel struct {
	DefaultNodeGroup types.Object `tfsdk:"default_node_group"`
}

var frontendAttrTypes = componentAttrTypes

type MetaSpecModel struct {
	DefaultNodeGroup types.Object `tfsdk:"default_node_group"`
	EtcdMetaStore    types.Object `tfsdk:"etcd_meta_store"`
}

var metaAttrTypes = map[string]attr.Type{
	"default_node_group": types.ObjectType{
		AttrTypes: defaultNodeGroup,
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
	ID      types.String `tfsdk:"id"`
	Region  types.String `tfsdk:"region"`
	Name    types.String `tfsdk:"name"`
	Version types.String `tfsdk:"version"`
	Spec    types.Object `tfsdk:"spec"`
}

type NodeGroupModel struct {
	CPU     types.String `tfsdk:"cpu"`
	Memory  types.String `tfsdk:"memory"`
	Replica types.Int64  `tfsdk:"replica"`
}

func (r *ClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *ClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	defauleNodeGroupAttribute := schema.SingleNestedAttribute{
		Attributes: map[string]schema.Attribute{
			"cpu": schema.StringAttribute{
				MarkdownDescription: "The CPU of the node",
				Required:            true,
			},
			"memory": schema.StringAttribute{
				MarkdownDescription: "The memory size in of the node",
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
		MarkdownDescription: clusterMarkdownDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The NsID (namespace id) of the cluster.",
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
					"It is used to fetch the image from the official image registry of RisingWave Labs." +
					"The newest stable version will be used if this field is not present.",
				Optional: true,
			},
			"spec": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"compute": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"default_node_group": defauleNodeGroupAttribute,
						},
						Required: true,
					},
					"compactor": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"default_node_group": defauleNodeGroupAttribute,
						},
						Required: true,
					},
					"frontend": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"default_node_group": defauleNodeGroupAttribute,
						},
						Required: true,
					},
					"meta": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"default_node_group": defauleNodeGroupAttribute,
							"etcd_meta_store": schema.SingleNestedAttribute{
								Attributes: map[string]schema.Attribute{
									"default_node_group": defauleNodeGroupAttribute,
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

func (r *ClusterResource) nodeGroupModelToComponentResource(
	ctx context.Context, diags *diag.Diagnostics, nodeGroup *NodeGroupModel, region string, tier apigen_mgmt.TierId, component string,
) *apigen_mgmt.ComponentResource {

	var (
		reqCPU     = nodeGroup.CPU.ValueString()
		reqMem     = nodeGroup.Memory.ValueString()
		reqReplica = nodeGroup.Replica.ValueInt64()
	)

	availableTypes, err := r.client.GetAvailableComponentTypes(ctx, region, tier, component)
	if err != nil {
		diags.AddError(
			"Failed to get available component types",
			err.Error(),
		)
		return nil
	}

	var candidates []apigen_mgmt.AvailableComponentType
	for _, availableType := range availableTypes {
		if availableType.Cpu == reqCPU && availableType.Memory == reqMem {
			candidates = append(candidates, availableType)
		}
	}

	if len(candidates) == 0 {
		var availableCfg []string
		for _, availableType := range availableTypes {
			availableCfg = append(availableCfg, fmt.Sprintf("(%s, %s)", availableType.Cpu, availableType.Memory))
		}
		errStr := "configuration (%s, %s) is not allowed for %s component in %s tier, available configurations are: %v"
		diags.AddError(
			"Invalid configuration",
			fmt.Sprintf(errStr, reqCPU, reqMem, component, tier, availableCfg),
		)
		return nil
	}

	maximumReplica := 0
	chosenType := apigen_mgmt.AvailableComponentType{}
	for _, candidate := range candidates {
		if candidate.Maximum > maximumReplica {
			maximumReplica = candidate.Maximum
			chosenType = candidate
		}
	}
	if reqReplica > int64(maximumReplica) {
		diags.AddError(
			"Invalid replica",
			fmt.Sprintf("requested replica is greater than maximum replica %d", maximumReplica),
		)
		return nil
	}

	return &apigen_mgmt.ComponentResource{
		ComponentTypeId: chosenType.Id,
		Replica:         int(reqReplica),
		Cpu:             chosenType.Cpu,
		Memory:          chosenType.Memory,
	}
}

func clusterToDataModel(cluster *apigen_mgmt.Tenant, data *ClusterModel) {
	data.Name = types.StringValue(cluster.TenantName)
	data.Version = types.StringValue(cluster.ImageTag)
	data.ID = types.StringValue(cluster.NsId.String())
	data.Region = types.StringValue(cluster.Region)

	data.Spec = types.ObjectValueMust(
		clusterSpecAttrTypes,
		map[string]attr.Value{
			"risingwave_config": types.StringValue(cluster.RwConfig),
			"compute": types.ObjectValueMust(
				computeAttrTypes,
				map[string]attr.Value{
					"default_node_group": types.ObjectValueMust(
						defaultNodeGroup,
						map[string]attr.Value{
							"cpu":     types.StringValue(cluster.Resources.Components.Compute.Cpu),
							"memory":  types.StringValue(cluster.Resources.Components.Compute.Memory),
							"replica": types.Int64Value(int64(cluster.Resources.Components.Compute.Replica)),
						},
					),
				},
			),
			"compactor": types.ObjectValueMust(
				compactorAttrTypes,
				map[string]attr.Value{
					"default_node_group": types.ObjectValueMust(
						defaultNodeGroup,
						map[string]attr.Value{
							"cpu":     types.StringValue(cluster.Resources.Components.Compactor.Cpu),
							"memory":  types.StringValue(cluster.Resources.Components.Compactor.Memory),
							"replica": types.Int64Value(int64(cluster.Resources.Components.Compactor.Replica)),
						},
					),
				},
			),
			"frontend": types.ObjectValueMust(
				frontendAttrTypes,
				map[string]attr.Value{
					"default_node_group": types.ObjectValueMust(
						defaultNodeGroup,
						map[string]attr.Value{
							"cpu":     types.StringValue(cluster.Resources.Components.Frontend.Cpu),
							"memory":  types.StringValue(cluster.Resources.Components.Frontend.Memory),
							"replica": types.Int64Value(int64(cluster.Resources.Components.Frontend.Replica)),
						},
					),
				},
			),
			"meta": types.ObjectValueMust(
				metaAttrTypes,
				map[string]attr.Value{
					"default_node_group": types.ObjectValueMust(
						defaultNodeGroup,
						map[string]attr.Value{
							"cpu":     types.StringValue(cluster.Resources.Components.Meta.Cpu),
							"memory":  types.StringValue(cluster.Resources.Components.Meta.Memory),
							"replica": types.Int64Value(int64(cluster.Resources.Components.Meta.Replica)),
						},
					),
					"etcd_meta_store": types.ObjectValueMust(
						etcdMetaStoreAttrTypes,
						map[string]attr.Value{
							"default_node_group": types.ObjectValueMust(
								defaultNodeGroup,
								map[string]attr.Value{
									"cpu":     types.StringValue(cluster.Resources.Components.Etcd.Cpu),
									"memory":  types.StringValue(cluster.Resources.Components.Etcd.Memory),
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

func (r *ClusterResource) dataModelToCluster(ctx context.Context, data *ClusterModel, cluster *apigen_mgmt.Tenant) diag.Diagnostics {
	diags := diag.Diagnostics{}
	objectAsOptions := basetypes.ObjectAsOptions{
		UnhandledUnknownAsEmpty: true,
		UnhandledNullAsEmpty:    true,
	}

	var (
		spec                      ClusterSpecModel
		compactorSpec             CompactorSpecModel
		compactorDefaultNodeGroup NodeGroupModel
		computeSpec               ComputeSpecModel
		computeDefaultNodeGroup   NodeGroupModel
		frontendSpec              FrontendSpecModel
		frontendDefaultNodeGroup  NodeGroupModel
		metaSpec                  MetaSpecModel
		metaDefaultNodeGroup      NodeGroupModel

		useEtcdMetaStore     bool
		etcdMetaStore        EtcdMetaStoreModel
		etcdDefaultNodeGroup NodeGroupModel
	)

	tflog.Trace(ctx, "parsing spec")
	diags.Append(data.Spec.As(ctx, &spec, objectAsOptions)...)

	tflog.Trace(ctx, "parsing compactorSpec")
	diags.Append(spec.CompactorSpec.As(ctx, &compactorSpec, objectAsOptions)...)
	diags.Append(compactorSpec.DefaultNodeGroup.As(ctx, &compactorDefaultNodeGroup, objectAsOptions)...)

	tflog.Trace(ctx, "parsing computeSpec")
	diags.Append(spec.ComputeSpec.As(ctx, &computeSpec, objectAsOptions)...)
	diags.Append(computeSpec.DefaultNodeGroup.As(ctx, &computeDefaultNodeGroup, objectAsOptions)...)

	tflog.Trace(ctx, "parsing frontendSpec")
	diags.Append(spec.FrontendSpec.As(ctx, &frontendSpec, objectAsOptions)...)
	diags.Append(frontendSpec.DefaultNodeGroup.As(ctx, &frontendDefaultNodeGroup, objectAsOptions)...)

	tflog.Trace(ctx, "parsing metaSpec")
	diags.Append(spec.MetaSpec.As(ctx, &metaSpec, objectAsOptions)...)
	diags.Append(metaSpec.DefaultNodeGroup.As(ctx, &metaDefaultNodeGroup, objectAsOptions)...)

	if !metaSpec.EtcdMetaStore.IsNull() {
		tflog.Trace(ctx, "parsing etcdMetaStore")
		useEtcdMetaStore = true
		diags.Append(metaSpec.EtcdMetaStore.As(ctx, &etcdMetaStore, objectAsOptions)...)
		diags.Append(etcdMetaStore.DefaultNodeGroup.As(ctx, &etcdDefaultNodeGroup, objectAsOptions)...)
	}

	if !useEtcdMetaStore {
		diags.AddError(
			"Missing meta store",
			"Meta store is required to setup the cluster.",
		)
		return diags
	}

	if !data.ID.IsUnknown() && !data.ID.IsNull() {
		nsId, err := uuid.Parse(data.ID.ValueString())
		if err != nil {
			diags.AddError(
				"Failed to parse nsid when mapping data to cluster model",
				fmt.Sprintf("Cannot parse cluster NsID: %s", data.ID.String()),
			)
			return diags
		}
		cluster.NsId = nsId
	}

	cluster.TenantName = data.Name.ValueString()
	cluster.ImageTag = data.Version.ValueString()
	cluster.Tier = DefaultTier
	cluster.RwConfig = spec.RisingWaveConfig.ValueString()
	cluster.EtcdConfig = etcdMetaStore.EtcdConfig.ValueString()
	cluster.Region = data.Region.ValueString()

	computeResource := r.nodeGroupModelToComponentResource(ctx, &diags, &computeDefaultNodeGroup, cluster.Region, cluster.Tier, "compute")
	compactorResource := r.nodeGroupModelToComponentResource(ctx, &diags, &compactorDefaultNodeGroup, cluster.Region, cluster.Tier, "compactor")
	frontendResource := r.nodeGroupModelToComponentResource(ctx, &diags, &frontendDefaultNodeGroup, cluster.Region, cluster.Tier, "frontend")
	metaResource := r.nodeGroupModelToComponentResource(ctx, &diags, &metaDefaultNodeGroup, cluster.Region, cluster.Tier, "meta")
	etcdResuorce := r.nodeGroupModelToComponentResource(ctx, &diags, &etcdDefaultNodeGroup, cluster.Region, cluster.Tier, "etcd")

	if diags.HasError() {
		return diags
	}

	cluster.Resources = apigen_mgmt.TenantResource{
		Components: apigen_mgmt.TenantResourceComponents{
			Compactor: compactorResource,
			Compute:   computeResource,
			Frontend:  frontendResource,
			Meta:      metaResource,
			Etcd:      *etcdResuorce,
		},
		ComputeFileCacheSizeGiB: DefaultComputeFileCacheSizeGB,
		EnableComputeFileCache:  DefaultEnableComputeFileCache,
		EtcdVolumeSizeGiB:       DefaultEtcdVolumeSizeGB,
	}

	return diags
}

func (r *ClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		region = data.Region.ValueString()
	)

	var cluster apigen_mgmt.Tenant

	if len(region) == 0 {
		resp.Diagnostics.AddError(
			"Invalid region",
			"Region is required",
		)
		return
	}

	resp.Diagnostics.Append(r.dataModelToCluster(ctx, &data, &cluster)...)

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
	nsID, err := uuid.Parse(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to parse nsid when reading cluster resource",
			fmt.Sprintf("Cannot parse cluster NsID: %s", data.ID.String()),
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
	return a.Cpu == b.Cpu &&
		a.Memory == b.Memory &&
		a.Replica == b.Replica
}

func (r *ClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var (
		data  ClusterModel
		state ClusterModel
	)

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// minimal identifiers for import state
	nsID, err := uuid.Parse(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to parse nsid when updating cluster resource",
			fmt.Sprintf("Cannot parse cluster NsID: %s", data.ID.String()),
		)
		return
	}

	var updated = apigen_mgmt.Tenant{}

	resp.Diagnostics.Append(r.dataModelToCluster(ctx, &data, &updated)...)

	previous, err := r.client.GetClusterByNsID(ctx, nsID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read cluster",
			err.Error(),
		)
		return
	}

	// assign ID as the ID is obtained by computing.
	data.ID = types.StringValue(previous.NsId.String())

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
	if previous.Region != updated.Region {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			"Region name cannot be changed",
		)
	}

	if !resourceEqual(&previous.Resources.Components.Etcd, &updated.Resources.Components.Etcd) {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			fmt.Sprintf("Etcd resource cannot be changed, previous: %v, updated: %v", previous.Resources.Components.Etcd, updated.Resources.Components.Etcd),
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
	nsID, err := uuid.Parse(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to parse nsid when deleting cluster resource",
			fmt.Sprintf("Cannot parse cluster NsID: %s", data.ID.String()),
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
			fmt.Sprintf("Failed to parse id: %s", req.ID),
		)
		return
	}
	if _, err := r.client.GetClusterByNsID(ctx, nsID); err != nil {
		resp.Diagnostics.AddError(
			"Unable to read cluster",
			err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), nsID.String())...)
}
