package provider

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/pkg/errors"

	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
)

// The tenant extensions live on the cluster resource rather than in resources of their own,
// because one of them reaches into the cluster's own spec: enabling serverless compaction
// scales the cluster's compactor to zero, since that is the point of the feature. Keeping the
// two in one resource is what lets the provider hold them apart.
//
// `spec.compactor.default_node_group.replica` stays what the practitioner declared, all the way
// through. The platform agrees with that reading: it records the count in
// `OriginalCompactorReplicas` -- a field its own spec marks as server-set -- and restores it
// when the extension is disabled. Zero is a state the extension holds the compactor in, not a
// new desired value, so the provider neither records it nor sends it back.
//
// The cost of keeping them here is that an extension that fails to enable during the apply that
// creates the cluster leaves the cluster created but the apply in error. Create writes the
// cluster to state before it touches an extension so that terraform knows about it -- without
// that the cluster would exist with nothing tracking it, and the next apply would be answered
// with `Cluster already exists`.
//
// Extensions are not available on a standalone cluster. The platform refuses even to report
// them, with a 412, so the cluster resource skips them entirely rather than turning every plan
// of every standalone cluster into an error.

type ClusterExtensionsModel struct {
	ServerlessCompaction types.Object `tfsdk:"serverless_compaction"`
	ServerlessBackfill   types.Object `tfsdk:"serverless_backfill"`
	IcebergCompaction    types.Object `tfsdk:"iceberg_compaction"`
}

type ServerlessCompactionModel struct {
	MaximumCompactionConcurrency types.Int64  `tfsdk:"maximum_compaction_concurrency"`
	Version                      types.String `tfsdk:"version"`
	Status                       types.String `tfsdk:"status"`
}

// ExtensionNodesModel is shared by the two extensions that run their own nodes.
type ExtensionNodesModel struct {
	ComponentTypeID types.String `tfsdk:"component_type_id"`
	Replica         types.Int64  `tfsdk:"replica"`
	CPU             types.String `tfsdk:"cpu"`
	Memory          types.String `tfsdk:"memory"`
	Status          types.String `tfsdk:"status"`
}

type IcebergCompactionModel struct {
	ComponentTypeID types.String `tfsdk:"component_type_id"`
	Replica         types.Int64  `tfsdk:"replica"`
	Config          types.String `tfsdk:"config"`
	CPU             types.String `tfsdk:"cpu"`
	Memory          types.String `tfsdk:"memory"`
	Status          types.String `tfsdk:"status"`
}

var serverlessCompactionAttrTypes = map[string]attr.Type{
	"maximum_compaction_concurrency": types.Int64Type,
	"version":                        types.StringType,
	"status":                         types.StringType,
}

var extensionNodesAttrTypes = map[string]attr.Type{
	"component_type_id": types.StringType,
	"replica":           types.Int64Type,
	"cpu":               types.StringType,
	"memory":            types.StringType,
	"status":            types.StringType,
}

var icebergCompactionAttrTypes = map[string]attr.Type{
	"component_type_id": types.StringType,
	"replica":           types.Int64Type,
	"config":            types.StringType,
	"cpu":               types.StringType,
	"memory":            types.StringType,
	"status":            types.StringType,
}

var clusterExtensionsAttrTypes = map[string]attr.Type{
	"serverless_compaction": types.ObjectType{AttrTypes: serverlessCompactionAttrTypes},
	"serverless_backfill":   types.ObjectType{AttrTypes: extensionNodesAttrTypes},
	"iceberg_compaction":    types.ObjectType{AttrTypes: icebergCompactionAttrTypes},
}

// clusterExtensionsAttribute is the `extensions` block of the cluster schema.
func clusterExtensionsAttribute() schema.SingleNestedAttribute {
	nodes := func(what string) map[string]schema.Attribute {
		return map[string]schema.Attribute{
			"component_type_id": schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf(
					"The component type the %s nodes run on, for example `p-1c4g`. It must be one of the "+
						"compute sizes the cluster's tier offers.", what),
				Required: true,
			},
			"replica": schema.Int64Attribute{
				MarkdownDescription: fmt.Sprintf("How many %s nodes to run. Must be greater than 0.", what),
				Required:            true,
				Validators:          []validator.Int64{positiveInt64Validator{subject: "replica"}},
			},
			"cpu": schema.StringAttribute{
				MarkdownDescription: "The CPU size of a node, resolved by the platform from the component type.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					extensionComputedString{dependsOn: "component_type_id"},
				},
			},
			"memory": schema.StringAttribute{
				MarkdownDescription: "The memory size of a node, resolved by the platform from the component type.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					extensionComputedString{dependsOn: "component_type_id"},
				},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "The status the platform reports, `Running` once the extension is in service.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					extensionComputedString{},
				},
			},
		}
	}

	icebergAttributes := nodes("compaction")
	icebergAttributes["config"] = schema.StringAttribute{
		MarkdownDescription: "The extension's configuration, as a **TOML** document. The platform parses it " +
			"with a TOML decoder and rejects anything else, so a JSON object is not accepted here. It is " +
			"stored as written, which lets terraform compare it verbatim; a heredoc keeps it readable.",
		Optional: true,
	}

	return schema.SingleNestedAttribute{
		MarkdownDescription: "The platform-managed extensions of the cluster. An extension is enabled while " +
			"its block is present and disabled when it is removed. They need a cluster with a separate " +
			"compute component, so none of them is available on a standalone cluster.",
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"serverless_compaction": schema.SingleNestedAttribute{
				MarkdownDescription: "Runs the cluster's compaction on platform-managed capacity rather than " +
					"on its own compactor nodes. While it is enabled the platform scales " +
					"`spec.compactor.default_node_group.replica` to zero and owns that value; it is restored " +
					"when the extension is removed.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"maximum_compaction_concurrency": schema.Int64Attribute{
						MarkdownDescription: "How many compaction tasks may run at once. Must be greater than 0.",
						Required:            true,
						Validators:          []validator.Int64{positiveInt64Validator{subject: "maximum compaction concurrency"}},
					},
					"version": schema.StringAttribute{
						MarkdownDescription: "The version of the compaction proxy. **Setting this is not " +
							"recommended.** Left alone, the platform runs the newest version and keeps it that " +
							"way; naming one here pins it, and a pinned version ages out with nothing to warn " +
							"you. It is here for the case where support asks you to hold a particular version.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.String{
							extensionComputedString{},
						},
					},
					"status": schema.StringAttribute{
						MarkdownDescription: "The status the platform reports, `Running` once the extension is in service.",
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							extensionComputedString{},
						},
					},
				},
			},
			"serverless_backfill": schema.SingleNestedAttribute{
				MarkdownDescription: "Runs the backfilling of new materialized views on dedicated nodes, so it " +
					"does not compete with the cluster's streaming work.",
				Optional:   true,
				Attributes: nodes("backfill"),
			},
			"iceberg_compaction": schema.SingleNestedAttribute{
				MarkdownDescription: "Runs compaction over the cluster's Iceberg tables on dedicated nodes.",
				Optional:            true,
				Attributes:          icebergAttributes,
			},
		},
	}
}

// positiveInt64Validator rejects a value the platform would reject anyway, at plan time rather
// than halfway through an apply.
type positiveInt64Validator struct {
	subject string
}

func (v positiveInt64Validator) Description(ctx context.Context) string {
	return "value must be greater than 0"
}

func (v positiveInt64Validator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v positiveInt64Validator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if got := req.ConfigValue.ValueInt64(); got <= 0 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			fmt.Sprintf("Invalid %s", v.subject),
			fmt.Sprintf("Expected a value greater than 0, got: %d", got),
		)
	}
}

// clusterIsStandalone reports whether the cluster runs a single node rather than separate
// components. The platform will not even report the extensions of such a cluster: the three GET
// handlers answer 412 before looking at anything else.
func clusterIsStandalone(cluster *apigen_mgmtv2.Tenant) bool {
	return cluster != nil && cluster.Resources.Components.Standalone != nil
}

// isEmpty reports whether no extension is asked for, which is the common case: a cluster
// without any is not charged an extra API call for them.
func (m ClusterExtensionsModel) isEmpty() bool {
	return m.ServerlessCompaction.IsNull() && m.ServerlessBackfill.IsNull() && m.IcebergCompaction.IsNull()
}

// extensionsOf reads the `extensions` object, treating a null one as all three absent.
func extensionsOf(ctx context.Context, obj types.Object, diags *diag.Diagnostics) ClusterExtensionsModel {
	empty := ClusterExtensionsModel{
		ServerlessCompaction: types.ObjectNull(serverlessCompactionAttrTypes),
		ServerlessBackfill:   types.ObjectNull(extensionNodesAttrTypes),
		IcebergCompaction:    types.ObjectNull(icebergCompactionAttrTypes),
	}
	if obj.IsNull() || obj.IsUnknown() {
		return empty
	}

	var model ClusterExtensionsModel
	diags.Append(obj.As(ctx, &model, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})...)
	if diags.HasError() {
		return empty
	}
	return model
}

// serverlessCompactionEnabled reports whether the configuration asks for the extension that
// takes the cluster's compactor away.
func serverlessCompactionEnabled(ctx context.Context, obj types.Object, diags *diag.Diagnostics) bool {
	ext := extensionsOf(ctx, obj, diags)
	return !ext.ServerlessCompaction.IsNull() && !ext.ServerlessCompaction.IsUnknown()
}

// applyExtensions brings the cluster's extensions to what the plan asks for, one at a time.
//
// The order is deliberate: what is being removed goes first, so a change that swaps one
// extension's nodes for another's does not ask the platform for both at once. The calls are
// sequential because the platform refuses a request that overlaps another workflow on the same
// tenant, and each of these is a workflow.
func applyExtensions(
	ctx context.Context,
	client cloudsdk.CloudClientInterface,
	nsID uuid.UUID,
	plan, state ClusterExtensionsModel,
	diags *diag.Diagnostics,
) error {
	type step struct {
		name    string
		planned types.Object
		current types.Object
		disable func() error
		apply   func(enabled bool) error
	}

	var compaction ServerlessCompactionModel
	if !plan.ServerlessCompaction.IsNull() {
		diags.Append(plan.ServerlessCompaction.As(ctx, &compaction, basetypes.ObjectAsOptions{})...)
	}
	var backfill ExtensionNodesModel
	if !plan.ServerlessBackfill.IsNull() {
		diags.Append(plan.ServerlessBackfill.As(ctx, &backfill, basetypes.ObjectAsOptions{})...)
	}
	var iceberg IcebergCompactionModel
	if !plan.IcebergCompaction.IsNull() {
		diags.Append(plan.IcebergCompaction.As(ctx, &iceberg, basetypes.ObjectAsOptions{})...)
	}
	if diags.HasError() {
		return nil
	}

	steps := []step{
		{
			name:    "serverless_compaction",
			planned: plan.ServerlessCompaction,
			current: state.ServerlessCompaction,
			disable: func() error { return client.DisableServerlessCompactionAwait(ctx, nsID) },
			apply: func(enabled bool) error {
				req := apigen_mgmtv2.TenantExtensionServerlessCompactionRequest{
					MaximumCompactionConcurrency: int(compaction.MaximumCompactionConcurrency.ValueInt64()),
				}
				if !compaction.Version.IsNull() && !compaction.Version.IsUnknown() {
					version := compaction.Version.ValueString()
					req.Version = &version
				}
				if enabled {
					return client.UpdateServerlessCompactionAwait(ctx, nsID, req)
				}
				return client.EnableServerlessCompactionAwait(ctx, nsID, req)
			},
		},
		{
			name:    "serverless_backfill",
			planned: plan.ServerlessBackfill,
			current: state.ServerlessBackfill,
			disable: func() error { return client.DisableServerlessBackfillAwait(ctx, nsID) },
			apply: func(enabled bool) error {
				req := apigen_mgmtv2.TenantExtensionServerlessBackfillRequest{
					Resources: apigen_mgmtv2.ComponentResourceRequest{
						ComponentTypeId: backfill.ComponentTypeID.ValueString(),
						Replica:         int(backfill.Replica.ValueInt64()),
					},
				}
				if enabled {
					return client.UpdateServerlessBackfillAwait(ctx, nsID, req)
				}
				return client.EnableServerlessBackfillAwait(ctx, nsID, req)
			},
		},
		{
			name:    "iceberg_compaction",
			planned: plan.IcebergCompaction,
			current: state.IcebergCompaction,
			disable: func() error { return client.DisableIcebergCompactionAwait(ctx, nsID) },
			apply: func(enabled bool) error {
				req := apigen_mgmtv2.PostTenantsNsIdExtensionsIcebergCompactionJSONRequestBody{
					Resources: &apigen_mgmtv2.ComponentResourceRequest{
						ComponentTypeId: iceberg.ComponentTypeID.ValueString(),
						Replica:         int(iceberg.Replica.ValueInt64()),
					},
				}
				if !iceberg.Config.IsNull() && !iceberg.Config.IsUnknown() {
					config := iceberg.Config.ValueString()
					req.Config = &config
				}
				if enabled {
					return client.UpdateIcebergCompactionAwait(ctx, nsID,
						apigen_mgmtv2.PutTenantsNsIdExtensionsIcebergCompactionJSONRequestBody(req))
				}
				return client.EnableIcebergCompactionAwait(ctx, nsID, req)
			},
		},
	}

	// removals first
	for _, s := range steps {
		if s.planned.IsNull() && !s.current.IsNull() {
			if err := s.disable(); err != nil && !errors.Is(err, cloudsdk.ErrExtensionDisabled) {
				return errors.Wrapf(err, "failed to disable the %s extension", s.name)
			}
		}
	}
	for _, s := range steps {
		if s.planned.IsNull() {
			continue
		}
		// An extension the plan leaves exactly as it is needs no request. Sending one anyway
		// would start a platform workflow for every extension on the cluster whenever anything
		// else about the cluster changed -- a version bump would restart all three.
		if s.planned.Equal(s.current) {
			continue
		}
		if err := s.apply(!s.current.IsNull()); err != nil {
			return errors.Wrapf(err, "failed to enable the %s extension", s.name)
		}
	}
	return nil
}

// readExtensions reports what the platform has, which is what the state must hold. An extension
// that is not enabled reads back as absent rather than as an error.
func readExtensions(ctx context.Context, client cloudsdk.CloudClientInterface, nsID uuid.UUID) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	compaction := types.ObjectNull(serverlessCompactionAttrTypes)
	if ext, err := client.GetServerlessCompaction(ctx, nsID); err == nil {
		value := ServerlessCompactionModel{
			MaximumCompactionConcurrency: types.Int64Null(),
			Version:                      types.StringNull(),
			Status:                       types.StringValue(ext.Status),
		}
		if ext.MaximumCompactionConcurrency != nil {
			value.MaximumCompactionConcurrency = types.Int64Value(int64(*ext.MaximumCompactionConcurrency))
		}
		if ext.Version != nil {
			value.Version = types.StringValue(*ext.Version)
		}
		obj, d := types.ObjectValueFrom(ctx, serverlessCompactionAttrTypes, value)
		diags.Append(d...)
		compaction = obj
	} else if !errors.Is(err, cloudsdk.ErrExtensionDisabled) {
		diags.AddError("Unable to read the serverless compaction extension", err.Error())
	}

	backfill := types.ObjectNull(extensionNodesAttrTypes)
	if ext, err := client.GetServerlessBackfill(ctx, nsID); err == nil {
		value := ExtensionNodesModel{Status: types.StringValue(ext.Status)}
		fillExtensionNodes(&value, ext.Resources)
		obj, d := types.ObjectValueFrom(ctx, extensionNodesAttrTypes, value)
		diags.Append(d...)
		backfill = obj
	} else if !errors.Is(err, cloudsdk.ErrExtensionDisabled) {
		diags.AddError("Unable to read the serverless backfill extension", err.Error())
	}

	iceberg := types.ObjectNull(icebergCompactionAttrTypes)
	if ext, err := client.GetIcebergCompaction(ctx, nsID); err == nil {
		nodes := ExtensionNodesModel{Status: types.StringValue(ext.Status)}
		fillExtensionNodes(&nodes, ext.Resources)
		value := IcebergCompactionModel{
			ComponentTypeID: nodes.ComponentTypeID,
			Replica:         nodes.Replica,
			Config:          types.StringNull(),
			CPU:             nodes.CPU,
			Memory:          nodes.Memory,
			Status:          nodes.Status,
		}
		if ext.Config != nil {
			value.Config = types.StringValue(*ext.Config)
		}
		obj, d := types.ObjectValueFrom(ctx, icebergCompactionAttrTypes, value)
		diags.Append(d...)
		iceberg = obj
	} else if !errors.Is(err, cloudsdk.ErrExtensionDisabled) {
		diags.AddError("Unable to read the iceberg compaction extension", err.Error())
	}

	if compaction.IsNull() && backfill.IsNull() && iceberg.IsNull() {
		return types.ObjectNull(clusterExtensionsAttrTypes), diags
	}

	obj, d := types.ObjectValue(clusterExtensionsAttrTypes, map[string]attr.Value{
		"serverless_compaction": compaction,
		"serverless_backfill":   backfill,
		"iceberg_compaction":    iceberg,
	})
	diags.Append(d...)
	return obj, diags
}

func fillExtensionNodes(value *ExtensionNodesModel, resources *apigen_mgmtv2.ComponentResource) {
	if resources == nil {
		value.ComponentTypeID = types.StringNull()
		value.Replica = types.Int64Null()
		value.CPU = types.StringNull()
		value.Memory = types.StringNull()
		return
	}
	value.ComponentTypeID = types.StringValue(resources.ComponentTypeId)
	value.Replica = types.Int64Value(int64(resources.Replica))
	value.CPU = types.StringValue(resources.Cpu)
	value.Memory = types.StringValue(resources.Memory)
}

// extensionComputedString keeps a computed attribute of an extension at what the platform last
// reported, so that a cluster whose extensions have not changed plans empty.
//
// Terraform plans a computed attribute as unknown unless the provider says otherwise, and an
// unknown is a diff: without this, every plan would offer to update `cpu`, `memory`, `status`
// and `version` even when nothing about the extension changed.
//
// dependsOn names a sibling attribute that the platform resolves this one from. When that
// sibling changes the value has to be planned as unknown again, or the platform's new answer
// would contradict a plan that promised the old one.
type extensionComputedString struct {
	dependsOn string
}

func (m extensionComputedString) Description(ctx context.Context) string {
	if m.dependsOn == "" {
		return "the value the platform last reported is kept"
	}
	return fmt.Sprintf("the value the platform last reported is kept unless %s changes", m.dependsOn)
}

func (m extensionComputedString) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m extensionComputedString) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Nothing to carry forward when the resource is being created or destroyed.
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}

	// An extension that is only now being enabled has no prior value, so the attribute stays
	// unknown. This is asked of the extension object rather than of the attribute, because an
	// attribute the platform reports as absent is a null the state legitimately holds -- the
	// serverless compaction version is one, when the platform does not name one -- and keeping
	// it unknown would leave a diff that never settles.
	var extension types.Object
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, req.Path.ParentPath(), &extension)...)
	if resp.Diagnostics.HasError() || extension.IsNull() {
		return
	}
	// A value the practitioner wrote is theirs, not the platform's.
	if !req.ConfigValue.IsNull() {
		return
	}

	if m.dependsOn != "" {
		sibling := req.Path.ParentPath().AtName(m.dependsOn)

		var planned, current types.String
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, sibling, &planned)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, sibling, &current)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !planned.Equal(current) {
			return
		}
	}

	resp.PlanValue = req.StateValue
}

// declaredCompactorReplica reads the compactor replica count out of a cluster's `spec`, which is
// the count the practitioner asked for.
func declaredCompactorReplica(ctx context.Context, spec types.Object, diags *diag.Diagnostics) (int, bool) {
	if spec.IsNull() || spec.IsUnknown() {
		return 0, false
	}

	var model ClusterSpecModel
	diags.Append(spec.As(ctx, &model, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})...)
	if diags.HasError() || model.CompactorSpec.IsNull() || model.CompactorSpec.IsUnknown() {
		return 0, false
	}

	var component struct {
		DefaultNodeGroup types.Object `tfsdk:"default_node_group"`
	}
	diags.Append(model.CompactorSpec.As(ctx, &component, basetypes.ObjectAsOptions{})...)
	if diags.HasError() || component.DefaultNodeGroup.IsNull() || component.DefaultNodeGroup.IsUnknown() {
		return 0, false
	}

	var group NodeGroupModel
	diags.Append(component.DefaultNodeGroup.As(ctx, &group, basetypes.ObjectAsOptions{})...)
	if diags.HasError() || group.Replica.IsNull() || group.Replica.IsUnknown() {
		return 0, false
	}
	return int(group.Replica.ValueInt64()), true
}

// keepDeclaredCompactor puts the declared compactor count back on a cluster the platform has
// answered with, so that the zero serverless compaction holds it at never reaches the state.
// Without this every plan would offer to restore the compactors, which would fight the
// extension, and the practitioner would have no way to write a configuration that settles.
func keepDeclaredCompactor(ctx context.Context, spec types.Object, cluster *apigen_mgmtv2.Tenant, diags *diag.Diagnostics) {
	if cluster == nil || cluster.Resources.Components.Compactor == nil {
		return
	}
	declared, ok := declaredCompactorReplica(ctx, spec, diags)
	if !ok || diags.HasError() {
		return
	}
	cluster.Resources.Components.Compactor.Replica = declared
}

// plannedCompactionConcurrency reports the concurrency the plan asks serverless compaction to
// run at, and whether the extension is asked for at all.
//
// A rescale has to carry this. The platform's resource endpoint treats an absent extension in
// the request as a request for zero concurrency, which disables it, so a change to the compute
// nodes that said nothing about compaction would turn compaction off.
func plannedCompactionConcurrency(ctx context.Context, extensions types.Object, diags *diag.Diagnostics) (int, bool) {
	ext := extensionsOf(ctx, extensions, diags)
	if diags.HasError() || ext.ServerlessCompaction.IsNull() || ext.ServerlessCompaction.IsUnknown() {
		return 0, false
	}

	var compaction ServerlessCompactionModel
	diags.Append(ext.ServerlessCompaction.As(ctx, &compaction, basetypes.ObjectAsOptions{})...)
	if diags.HasError() || compaction.MaximumCompactionConcurrency.IsNull() ||
		compaction.MaximumCompactionConcurrency.IsUnknown() {
		return 0, false
	}
	return int(compaction.MaximumCompactionConcurrency.ValueInt64()), true
}

// ModifyPlan refuses to change the compactor while serverless compaction runs.
//
// The extension holds the compactor at zero and remembers the count to give back in
// `OriginalCompactorReplicas`, which the platform sets when the extension is enabled and offers
// no way to revise. A new count while the extension runs would therefore be recorded by
// terraform and forgotten by the platform: nothing would happen at the time, and disabling the
// extension later would restore the old count and produce a diff nobody asked for.
//
// Rather than accept a value that quietly does not take effect, the plan is refused, and it says
// what to do instead. The check is against the state, since that is what the platform has -- the
// compactor of a cluster whose extension this apply is about to enable is still the cluster's
// own, and can be changed freely.
func (r *ClusterResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing to check when the resource is being created or destroyed.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state, plan ClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !serverlessCompactionEnabled(ctx, state.Extensions, &resp.Diagnostics) || resp.Diagnostics.HasError() {
		return
	}

	current, hasCurrent := declaredCompactorReplica(ctx, state.Spec, &resp.Diagnostics)
	planned, hasPlanned := declaredCompactorReplica(ctx, plan.Spec, &resp.Diagnostics)
	if resp.Diagnostics.HasError() || !hasCurrent || !hasPlanned || current == planned {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("spec").AtName("compactor").AtName("default_node_group").AtName("replica"),
		"The compactor cannot be resized while serverless compaction is enabled",
		fmt.Sprintf(
			"The extension holds the compactor at zero replicas and restores %d when it is disabled, "+
				"a count the platform records at the time it is enabled and will not revise. Asking for %d "+
				"now would be recorded by terraform and ignored by the platform. Remove the "+
				"`extensions.serverless_compaction` block, apply, and then resize the compactor.",
			current, planned,
		),
	)
}
