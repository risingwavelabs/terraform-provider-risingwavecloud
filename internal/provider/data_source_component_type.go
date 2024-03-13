package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"

	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
)

const (
	ComponentCompute   = "compute"
	ComponentCompactor = "compactor"
	ComponentFrontend  = "frontend"
	ComponentMeta      = "meta"
	ComponentEtcd      = "etcd"
)

var (
	ComponentMenu = fmt.Sprintf("`%s`, `%s`, `%s`, `%s`, `%s`",
		ComponentCompute,
		ComponentCompactor,
		ComponentFrontend,
		ComponentMeta,
		ComponentEtcd,
	)
)

var (
	DefaultTier = apigen_mgmt.Standard
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ComponentTypeDataSource{}

func NewComponentTypeDataSource() datasource.DataSource {
	return &ComponentTypeDataSource{}
}

type ComponentTypeDataSource struct {
	client cloudsdk.CloudClientInterface
}

type ComponentTypeDataSourceModel struct {
	Platform  types.String `tfsdk:"platform"`
	Region    types.String `tfsdk:"region"`
	Component types.String `tfsdk:"component"`
	VCPU      types.Int64  `tfsdk:"vcpu"`
	MemoryGiB types.Int64  `tfsdk:"memory_gib"`
	ID        types.String `tfsdk:"id"`
}

func (d *ComponentTypeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_component_type"
}

func (d *ComponentTypeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The type of the component of the RisingWave cluster",
		Attributes: map[string]schema.Attribute{
			"platform": schema.StringAttribute{
				Required: true,
			},
			"region": schema.StringAttribute{
				Required: true,
			},
			"vcpu": schema.Int64Attribute{
				MarkdownDescription: "The number of the virtual CPU cores",
				Required:            true,
			},
			"memory_gib": schema.Int64Attribute{
				MarkdownDescription: "Memory size in GiB",
				Required:            true,
			},
			"component": schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf(
					"The component in a RisingWave cluster. Valid values are: %s", ComponentMenu,
				),
				Required: true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The id of the RisingWave cluster. Valid values are",
				Computed:            true,
			},
		},
	}
}

func (d *ComponentTypeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(cloudsdk.CloudClientInterface)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected cloudsdk.AccountServiceClientInterface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
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
	var (
		platform  = data.Platform.ValueString()
		region    = data.Region.ValueString()
		component = data.Component.ValueString()
		vCPU      = data.VCPU.ValueInt64()
		memoryGiB = data.MemoryGiB.ValueInt64()
		tier      = DefaultTier
	)

	if len(platform) == 0 {
		resp.Diagnostics.AddError(
			"Missing platform",
			"Platform is required to setup the provider.",
		)
		return
	}

	if len(region) == 0 {
		resp.Diagnostics.AddError(
			"Missing region",
			"Region is required to setup the provider.",
		)
		return
	}

	if len(component) == 0 {
		resp.Diagnostics.AddError(
			"Missing component",
			fmt.Sprintf("Component is required to setup the provider. Valid values are: %s", ComponentMenu),
		)
		return
	}

	availableComponentTypes, err := d.client.GetAvailableComponentTypes(ctx, region, tier, component)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to get available component types",
			err.Error(),
		)
		return
	}

	ok := false
	for _, c := range availableComponentTypes {
		if fmt.Sprintf("%d", vCPU) == c.Cpu && fmt.Sprintf("%d GB", memoryGiB) == c.Memory {
			data.ID = types.StringValue(c.Id)
			ok = true
			break
		}
	}

	if !ok {
		resp.Diagnostics.AddError(
			"Cannot found the corresponding component type",
			fmt.Sprintf(
				"The component type %s with CPU %d cores and memory %d GB is not available for the tier %s",
				component, vCPU, memoryGiB, tier,
			),
		)
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
