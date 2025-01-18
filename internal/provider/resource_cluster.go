package provider

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"golang.org/x/mod/semver"

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
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/ptr"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/wait"
)

var (
	DefaultComputeFileCacheSizeGB = 20
	DefaultEtcdVolumeSizeGB       = 16
)

// Assert provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClusterResource{}
var _ resource.ResourceWithImportState = &ClusterResource{}

func NewClusterResource() resource.Resource {
	return &ClusterResource{
		dataHelper: &DataExtractHelper{},
	}
}

type ClusterResource struct {
	client     cloudsdk.CloudClientInterface
	dataHelper DataExtractHelperInterface
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

type PostgresqlMetaStoreModel struct {
	Resource types.Object `tfsdk:"resource"`
	SizeGb   types.Int64  `tfsdk:"size_gb"`
}

type AwsRdsMetaStoreModel struct {
	InstanceClass types.String `tfsdk:"instance_class"`
	SizeGb        types.Int64  `tfsdk:"size_gb"`
}

type GcpCloudsqlMetaStoreModel struct {
	Tier   types.String `tfsdk:"tier"`
	SizeGb types.Int64  `tfsdk:"size_gb"`
}

type AzrPostgresMetaStoreModel struct {
	Sku    types.String `tfsdk:"sku"`
	SizeGb types.Int64  `tfsdk:"size_gb"`
}

var metaStorePostgresqlAttrTypes = map[string]attr.Type{
	"resource": types.ObjectType{
		AttrTypes: defaultNodeGroup,
	},
	"size_gb": types.Int64Type,
}

var metaStoreAwsRdsAttrTypes = map[string]attr.Type{
	"instance_class": types.StringType,
	"size_gb":        types.Int64Type,
}

var metaStoreGcpCloudsqlAttrTypes = map[string]attr.Type{
	"tier":    types.StringType,
	"size_gb": types.Int64Type,
}

var metaStoreAzrPostgresAttrTypes = map[string]attr.Type{
	"sku":     types.StringType,
	"size_gb": types.Int64Type,
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

type MetaStoreModel struct {
	Postgresql  types.Object `tfsdk:"postgresql"`
	SharingPg   types.Object `tfsdk:"sharing_pg"`
	AwsRds      types.Object `tfsdk:"aws_rds"`
	GcpCloudsql types.Object `tfsdk:"gcp_cloudsql"`
	AzrPostgres types.Object `tfsdk:"azr_postgres"`
}

var metaStoreAttrTypes = map[string]attr.Type{
	"postgresql": types.ObjectType{
		AttrTypes: metaStorePostgresqlAttrTypes,
	},
	"sharing_pg": types.ObjectType{},
	"aws_rds": types.ObjectType{
		AttrTypes: metaStoreAwsRdsAttrTypes,
	},
	"gcp_cloudsql": types.ObjectType{
		AttrTypes: metaStoreGcpCloudsqlAttrTypes,
	},
	"azr_postgres": types.ObjectType{
		AttrTypes: metaStoreAzrPostgresAttrTypes,
	},
}

type MetaSpecModel struct {
	DefaultNodeGroup types.Object `tfsdk:"default_node_group"`
	EtcdMetaStore    types.Object `tfsdk:"etcd_meta_store"`
	MetaStore        types.Object `tfsdk:"meta_store"`
}

var metaAttrTypes = map[string]attr.Type{
	"default_node_group": types.ObjectType{
		AttrTypes: defaultNodeGroup,
	},
	"etcd_meta_store": types.ObjectType{
		AttrTypes: etcdMetaStoreAttrTypes,
	},
	"meta_store": types.ObjectType{
		AttrTypes: metaStoreAttrTypes,
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

type BYOCModel struct {
	Env types.String `tfsdk:"env"`
}

var byocAttrTypes = map[string]attr.Type{
	"env": types.StringType,
}

type ClusterModel struct {
	ID      types.String `tfsdk:"id"`
	Tier    types.String `tfsdk:"tier"`
	Region  types.String `tfsdk:"region"`
	Name    types.String `tfsdk:"name"`
	Version types.String `tfsdk:"version"`
	BYOC    types.Object `tfsdk:"byoc"`
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
		Description:         "A managed RisingWave Cluster on the RisingWave Cloud platform",
		MarkdownDescription: clusterMarkdownDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The NsID (namespace id) of the cluster.",
				Computed:            true,
			},
			"tier": schema.StringAttribute{
				MarkdownDescription: "The tier of your RisingWave cluster. When creating a new cluster, the value is `standard`.",
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
			"byoc": schema.SingleNestedAttribute{
				MarkdownDescription: "The BYOC (Bring Your Own Cloud) configuration of the cluster. " +
					"These fields are only used in BYOC clusters.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"env": schema.StringAttribute{
						MarkdownDescription: "The environment of BYOC clusters. " +
							"This field is only used in BYOC clusters.",
						Required: true,
					},
				},
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
								Optional:    true,
								Description: "The etcd meta store is no longer supported in new RisingWave versions, this field is kept for compatibility, please remove it if your RisingWave version is above v2.1.0",
							},
							"meta_store": schema.SingleNestedAttribute{
								Description: "The information of the meta store of the cluster",
								Attributes: map[string]schema.Attribute{
									"postgresql": schema.SingleNestedAttribute{
										Attributes: map[string]schema.Attribute{
											"resource": defauleNodeGroupAttribute,
											"size_gb": schema.Int64Attribute{
												MarkdownDescription: "The size of the PostgreSQL instance",
												Required:            true,
											},
										},
										Optional: true,
									},
									"sharing_pg": schema.SingleNestedAttribute{
										Attributes: map[string]schema.Attribute{},
										Optional:   true,
									},
									"aws_rds": schema.SingleNestedAttribute{
										Attributes: map[string]schema.Attribute{
											"instance_class": schema.StringAttribute{
												MarkdownDescription: "The instance class of the AWS RDS instance",
												Required:            true,
											},
											"size_gb": schema.Int64Attribute{
												MarkdownDescription: "The size of the AWS RDS instance",
												Required:            true,
											},
										},
										Optional: true,
									},
									"gcp_cloudsql": schema.SingleNestedAttribute{
										Attributes: map[string]schema.Attribute{
											"tier": schema.StringAttribute{
												MarkdownDescription: "The tier of the GCP CloudSQL instance",
												Required:            true,
											},
											"size_gb": schema.Int64Attribute{
												MarkdownDescription: "The size of the GCP CloudSQL instance",
												Required:            true,
											},
										},
										Optional: true,
									},
									"azr_postgres": schema.SingleNestedAttribute{
										Attributes: map[string]schema.Attribute{
											"sku": schema.StringAttribute{
												MarkdownDescription: "The SKU of the Azure PostgreSQL instance",
												Required:            true,
											},
											"size_gb": schema.Int64Attribute{
												MarkdownDescription: "The size of the Azure PostgreSQL instance",
												Required:            true,
											},
										},
										Optional: true,
									},
								},
								Computed: true,
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

func clusterToDataModel(cluster *apigen_mgmt.Tenant, data *ClusterModel) diag.Diagnostics {
	diags := diag.Diagnostics{}

	data.Name = types.StringValue(cluster.TenantName)
	data.Version = types.StringValue(cluster.ImageTag)
	data.ID = types.StringValue(cluster.NsId.String())
	data.Region = types.StringValue(cluster.Region)
	data.Tier = types.StringValue(string(cluster.Tier))

	if cluster.Tier == apigen_mgmt.BYOC {
		if cluster.ClusterName == nil {
			diags.AddError(
				"Missing BYOC env name",
				"The clusterName field is missing when the tier is BYOC",
			)
			return diags
		}
		data.BYOC = types.ObjectValueMust(byocAttrTypes, map[string]attr.Value{
			"env": types.StringValue(*cluster.ClusterName),
		})
	}

	specMetaObj := map[string]attr.Value{
		"default_node_group": types.ObjectValueMust(
			defaultNodeGroup,
			map[string]attr.Value{
				"cpu":     types.StringValue(cluster.Resources.Components.Meta.Cpu),
				"memory":  types.StringValue(cluster.Resources.Components.Meta.Memory),
				"replica": types.Int64Value(int64(cluster.Resources.Components.Meta.Replica)),
			},
		),
	}

	if cluster.Resources.MetaStore != nil && cluster.Resources.MetaStore.Type == apigen_mgmt.Etcd {
		specMetaObj["etcd_meta_store"] = types.ObjectValueMust(
			etcdMetaStoreAttrTypes,
			map[string]attr.Value{
				"default_node_group": types.ObjectValueMust(defaultNodeGroup, map[string]attr.Value{
					"cpu":     types.StringValue(cluster.Resources.MetaStore.Etcd.Resource.Cpu),
					"memory":  types.StringValue(cluster.Resources.MetaStore.Etcd.Resource.Memory),
					"replica": types.Int64Value(int64(cluster.Resources.MetaStore.Etcd.Resource.Replica)),
				}),
				"etcd_config": types.StringValue(cluster.EtcdConfig),
			},
		)
		specMetaObj["meta_store"] = types.ObjectNull(metaStoreAttrTypes)
	} else if cluster.Resources.MetaStore != nil {
		metaStoreObj := map[string]attr.Value{
			"postgresql":   types.ObjectNull(metaStorePostgresqlAttrTypes),
			"sharing_pg":   types.ObjectNull(map[string]attr.Type{}),
			"aws_rds":      types.ObjectNull(metaStoreAwsRdsAttrTypes),
			"gcp_cloudsql": types.ObjectNull(metaStoreGcpCloudsqlAttrTypes),
			"azr_postgres": types.ObjectNull(metaStoreAzrPostgresAttrTypes),
		}

		switch cluster.Resources.MetaStore.Type {
		case apigen_mgmt.Postgresql:
			metaStoreObj["postgresql"] = types.ObjectValueMust(metaStorePostgresqlAttrTypes, map[string]attr.Value{
				"resource": types.ObjectValueMust(defaultNodeGroup, map[string]attr.Value{
					"cpu":     types.StringValue(cluster.Resources.MetaStore.Postgresql.Resource.Cpu),
					"memory":  types.StringValue(cluster.Resources.MetaStore.Postgresql.Resource.Memory),
					"replica": types.Int64Value(int64(cluster.Resources.MetaStore.Postgresql.Resource.Replica)),
				}),
				"size_gb": types.Int64Value(int64(cluster.Resources.MetaStore.Postgresql.SizeGb)),
			})
		case apigen_mgmt.SharingPg:
			metaStoreObj["sharing_pg"] = types.ObjectValueMust(map[string]attr.Type{}, map[string]attr.Value{})
		case apigen_mgmt.AwsRds:
			metaStoreObj["aws_rds"] = types.ObjectValueMust(metaStoreAwsRdsAttrTypes, map[string]attr.Value{
				"instance_class": types.StringValue(cluster.Resources.MetaStore.AwsRds.InstanceClass),
				"size_gb":        types.Int64Value(int64(cluster.Resources.MetaStore.AwsRds.SizeGb)),
			})
		case apigen_mgmt.GcpCloudsql:
			metaStoreObj["gcp_cloudsql"] = types.ObjectValueMust(metaStoreGcpCloudsqlAttrTypes, map[string]attr.Value{
				"tier":    types.StringValue(cluster.Resources.MetaStore.GcpCloudsql.Tier),
				"size_gb": types.Int64Value(int64(cluster.Resources.MetaStore.GcpCloudsql.SizeGb)),
			})
		case apigen_mgmt.AzrPostgres:
			metaStoreObj["azr_postgres"] = types.ObjectValueMust(metaStoreAzrPostgresAttrTypes, map[string]attr.Value{
				"sku":     types.StringValue(cluster.Resources.MetaStore.AzrPostgres.Sku),
				"size_gb": types.Int64Value(int64(cluster.Resources.MetaStore.AzrPostgres.SizeGb)),
			})
		}

		specMetaObj["meta_store"] = types.ObjectValueMust(metaStoreAttrTypes, metaStoreObj)
		specMetaObj["etcd_meta_store"] = types.ObjectNull(etcdMetaStoreAttrTypes)
	}

	specObj := map[string]attr.Value{
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
		"meta": types.ObjectValueMust(metaAttrTypes, specMetaObj),
	}

	data.Spec = types.ObjectValueMust(
		clusterSpecAttrTypes,
		specObj,
	)

	return diags
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
		byoc                      BYOCModel

		metaStoreSpec      MetaStoreModel
		postgresqlSpec     PostgresqlMetaStoreModel
		postgresqlResource NodeGroupModel

		awsRdsSpec      AwsRdsMetaStoreModel
		gcpCloudsqlSpec GcpCloudsqlMetaStoreModel
		azrPostgresSpec AzrPostgresMetaStoreModel

		useEtcdMetaStore        bool
		usePostgresqlMetaStore  bool
		useSharingPgMetaStore   bool
		useAwsRdsMetaStore      bool
		useGcpCloudsqlMetaStore bool
		useAzrPostgresMetaStore bool

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

	if !metaSpec.MetaStore.IsNull() {
		diags.Append(metaSpec.MetaStore.As(ctx, &metaStoreSpec, objectAsOptions)...)
		if !metaStoreSpec.Postgresql.IsNull() {
			diags.Append(metaStoreSpec.Postgresql.As(ctx, &postgresqlSpec, objectAsOptions)...)
			diags.Append(postgresqlSpec.Resource.As(ctx, &postgresqlResource, objectAsOptions)...)
			usePostgresqlMetaStore = true
		}
		if !metaStoreSpec.SharingPg.IsNull() {
			useSharingPgMetaStore = true
		}
		if !metaStoreSpec.AwsRds.IsNull() {
			diags.Append(metaStoreSpec.AwsRds.As(ctx, &awsRdsSpec, objectAsOptions)...)
			useAwsRdsMetaStore = true
		}
		if !metaStoreSpec.GcpCloudsql.IsNull() {
			diags.Append(metaStoreSpec.GcpCloudsql.As(ctx, &gcpCloudsqlSpec, objectAsOptions)...)
			useGcpCloudsqlMetaStore = true
		}
		if !metaStoreSpec.AzrPostgres.IsNull() {
			diags.Append(metaStoreSpec.AzrPostgres.As(ctx, &azrPostgresSpec, objectAsOptions)...)
			useAzrPostgresMetaStore = true
		}
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

	if !data.BYOC.IsNull() && !data.BYOC.IsUnknown() {
		diags.Append(data.BYOC.As(ctx, &byoc, objectAsOptions)...)
		cluster.ClusterName = ptr.Ptr(byoc.Env.ValueString())
	}

	if semver.Compare(data.Version.ValueString(), "v2.1.0") >= 0 && useEtcdMetaStore {
		diags.AddError(
			"Invalid etcd meta store",
			"Etcd meta store is not supported for version v2.1.0 and above, please remove the etcd meta store from the spec if you are using version v2.1.0 or above",
		)
		return diags
	}

	cluster.TenantName = data.Name.ValueString()
	cluster.ImageTag = data.Version.ValueString()
	cluster.Tier = apigen_mgmt.TierId(data.Tier.ValueString())

	cluster.RwConfig = spec.RisingWaveConfig.ValueString()
	cluster.Region = data.Region.ValueString()

	computeResource := r.nodeGroupModelToComponentResource(ctx, &diags, &computeDefaultNodeGroup, cluster.Region, cluster.Tier, "compute")
	compactorResource := r.nodeGroupModelToComponentResource(ctx, &diags, &compactorDefaultNodeGroup, cluster.Region, cluster.Tier, "compactor")
	frontendResource := r.nodeGroupModelToComponentResource(ctx, &diags, &frontendDefaultNodeGroup, cluster.Region, cluster.Tier, "frontend")
	metaResource := r.nodeGroupModelToComponentResource(ctx, &diags, &metaDefaultNodeGroup, cluster.Region, cluster.Tier, "meta")

	if diags.HasError() {
		return diags
	}

	cluster.Resources = apigen_mgmt.TenantResource{
		Components: apigen_mgmt.TenantResourceComponents{
			Compactor: compactorResource,
			Compute:   computeResource,
			Frontend:  frontendResource,
			Meta:      metaResource,
		},
		ComputeCache: apigen_mgmt.TenantResourceComputeCache{
			SizeGb: DefaultComputeFileCacheSizeGB,
		},
	}

	if useEtcdMetaStore {
		etcdResuorce := r.nodeGroupModelToComponentResource(ctx, &diags, &etcdDefaultNodeGroup, cluster.Region, cluster.Tier, "etcd")
		cluster.Resources.MetaStore = &apigen_mgmt.TenantResourceMetaStore{
			Type: apigen_mgmt.Etcd,
			Etcd: &apigen_mgmt.MetaStoreEtcd{
				Resource: *etcdResuorce,
				SizeGb:   DefaultEtcdVolumeSizeGB,
			},
		}
		cluster.EtcdConfig = etcdMetaStore.EtcdConfig.ValueString()
	}

	if usePostgresqlMetaStore {
		cluster.Resources.MetaStore = &apigen_mgmt.TenantResourceMetaStore{
			Type: apigen_mgmt.Postgresql,
			Postgresql: &apigen_mgmt.MetaStorePostgreSql{
				Resource: *r.nodeGroupModelToComponentResource(ctx, &diags, &postgresqlResource, cluster.Region, cluster.Tier, "postgresql"),
				SizeGb:   int(postgresqlSpec.SizeGb.ValueInt64()),
			},
		}
	}

	if useSharingPgMetaStore {
		cluster.Resources.MetaStore = &apigen_mgmt.TenantResourceMetaStore{
			Type:      apigen_mgmt.SharingPg,
			SharingPg: &apigen_mgmt.MetaStoreSharingPg{},
		}
	}

	if useAwsRdsMetaStore {
		cluster.Resources.MetaStore = &apigen_mgmt.TenantResourceMetaStore{
			Type: apigen_mgmt.AwsRds,
			AwsRds: &apigen_mgmt.MetaStoreAwsRds{
				InstanceClass: awsRdsSpec.InstanceClass.ValueString(),
				SizeGb:        int(awsRdsSpec.SizeGb.ValueInt64()),
			},
		}
	}

	if useGcpCloudsqlMetaStore {
		cluster.Resources.MetaStore = &apigen_mgmt.TenantResourceMetaStore{
			Type: apigen_mgmt.GcpCloudsql,
			GcpCloudsql: &apigen_mgmt.MetaStoreGcpCloudSql{
				Tier:   gcpCloudsqlSpec.Tier.ValueString(),
				SizeGb: int(gcpCloudsqlSpec.SizeGb.ValueInt64()),
			},
		}
	}

	if useAzrPostgresMetaStore {
		cluster.Resources.MetaStore = &apigen_mgmt.TenantResourceMetaStore{
			Type: apigen_mgmt.AzrPostgres,
			AzrPostgres: &apigen_mgmt.MetaStoreAzrPostgres{
				Sku:    azrPostgresSpec.Sku.ValueString(),
				SizeGb: int(azrPostgresSpec.SizeGb.ValueInt64()),
			},
		}
	}

	return diags
}

func (r *ClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterModel

	resp.Diagnostics.Append(r.dataHelper.Get(ctx, &req.Plan, &data)...)
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

	if data.Tier.IsNull() || data.Tier.IsUnknown() {
		if data.BYOC.IsNull() || data.BYOC.IsUnknown() {
			data.Tier = types.StringValue(string(apigen_mgmt.Standard))
		} else {
			data.Tier = types.StringValue(string(apigen_mgmt.BYOC))
		}
	}

	resp.Diagnostics.Append(r.dataModelToCluster(ctx, &data, &cluster)...)

	if resp.Diagnostics.HasError() {
		return
	}

	c, err := r.client.GetClusterByRegionAndName(ctx, region, cluster.TenantName)
	if err != nil {
		if errors.Is(err, cloudsdk.ErrClusterNotFound) {
			// no previous cluster found, continue the creation
		} else {
			// abort on unknown errors
			resp.Diagnostics.AddError(
				"Failed to get cluster",
				err.Error(),
			)
			return
		}
	} else {
		// a healthy cluster already exists
		if c.Status != apigen_mgmt.Failed {
			resp.Diagnostics.AddError(
				"Cluster already exists",
				fmt.Sprintf("Cluster with the name %s already exists in the region %s", c.TenantName, region),
			)
			return
		}
		// delete the failed cluster
		if err := r.client.DeleteClusterByNsIDAwait(ctx, c.NsId); err != nil {
			resp.Diagnostics.AddError(
				"Failed to delete failed cluster before creation",
				err.Error(),
			)
			return
		}
	}

	var tenantReq = apigen_mgmt.TenantRequestRequestBody{}
	tenantReq.ClusterName = cluster.ClusterName
	tenantReq.TenantName = cluster.TenantName
	tenantReq.ImageTag = &cluster.ImageTag
	tenantReq.Tier = &cluster.Tier
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
		},
		ComputeFileCacheSizeGiB: cluster.Resources.ComputeCache.SizeGb,
	}

	if cluster.Resources.MetaStore != nil && cluster.Resources.MetaStore.Type == apigen_mgmt.Etcd {
		tenantReq.Resources.MetaStore = &apigen_mgmt.TenantResourceRequestMetaStore{
			Type: apigen_mgmt.Etcd,
			Etcd: &apigen_mgmt.TenantResourceRequestMetaStoreEtcd{
				ComponentTypeId: cluster.Resources.MetaStore.Etcd.Resource.ComponentTypeId,
				Replica:         cluster.Resources.MetaStore.Etcd.Resource.Replica,
				SizeGb:          cluster.Resources.MetaStore.Etcd.SizeGb,
			},
		}
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

	resp.Diagnostics.Append(clusterToDataModel(createdCluster, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Info(ctx, fmt.Sprintf("cluster created, UUID: %s", createdCluster.NsId))

	// Save data into Terraform state
	resp.Diagnostics.Append(r.dataHelper.Set(ctx, &resp.State, &data)...)
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

	if cluster.Tier != apigen_mgmt.Standard && cluster.Tier != apigen_mgmt.Invited && cluster.Tier != apigen_mgmt.BYOC {
		resp.Diagnostics.AddError(
			"Invalid tier",
			"Supported tiers are: Standard, Invited, BYOC",
		)
		return
	}

	resp.Diagnostics.Append(clusterToDataModel(cluster, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func metaStoreEqual(a, b *apigen_mgmt.TenantResourceMetaStore) bool {
	if a == nil || b == nil {
		return true
	}

	if a.Type != b.Type {
		return false
	}

	if a.Type == apigen_mgmt.Etcd {
		return resourceEqual(&a.Etcd.Resource, &b.Etcd.Resource) && a.Etcd.SizeGb == b.Etcd.SizeGb
	}

	return false
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

	previous, err := r.client.GetClusterByNsID(ctx, nsID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read cluster",
			err.Error(),
		)
		return
	}

	var updated = apigen_mgmt.Tenant{}

	// assign computed fields to updated
	data.ID = types.StringValue(previous.NsId.String())
	data.Tier = types.StringValue(string(previous.Tier))

	resp.Diagnostics.Append(r.dataModelToCluster(ctx, &data, &updated)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// immutable fields
	if previous.Tier != updated.Tier {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			"Tier cannot be changed",
		)
	}

	// only check clusterName for BYOC
	if previous.Tier == apigen_mgmt.BYOC {
		if previous.ClusterName == nil && updated.ClusterName != nil {
			resp.Diagnostics.AddError(
				"Cannot update immutable field",
				fmt.Sprintf("Cluster name cannot be changed, previous is not set, now is %s", *updated.ClusterName),
			)
		}
		if previous.ClusterName != nil && updated.ClusterName == nil {
			resp.Diagnostics.AddError(
				"Cannot update immutable field",
				fmt.Sprintf("Cluster name cannot be changed, previous is %s, now is not set", *previous.ClusterName),
			)
		}
		if (previous.ClusterName != nil && updated.ClusterName != nil) && (*previous.ClusterName != *updated.ClusterName) {
			resp.Diagnostics.AddError(
				"Cannot update immutable field",
				fmt.Sprintf("Cluster name cannot be changed, previous: %s, updated: %s", *previous.ClusterName, *updated.ClusterName),
			)
		}
	}

	if previous.TenantName != updated.TenantName {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			fmt.Sprintf("Tenant name cannot be changed, previous: %s, updated: %s", previous.TenantName, updated.TenantName),
		)
	}
	if previous.Region != updated.Region {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			fmt.Sprintf("Region cannot be changed, previous: %s, updated: %s", previous.Region, updated.Region),
		)
	}

	if !metaStoreEqual(previous.Resources.MetaStore, updated.Resources.MetaStore) {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			fmt.Sprintf("metastore cannot be changed, previous: %v, updated: %v", previous.Resources.MetaStore, updated.Resources.MetaStore),
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

	// Get the latest cluster state and save it to state
	now, err := r.client.GetClusterByNsID(ctx, nsID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read cluster",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(clusterToDataModel(now, &data)...)
	if resp.Diagnostics.HasError() {
		return
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
