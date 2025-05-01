package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/zakhar-gulchak/terraform-provider-fly/machineapi"
	"github.com/zakhar-gulchak/terraform-provider-fly/providerstate"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &flyVolumeResource{}
var _ resource.ResourceWithConfigure = &flyVolumeResource{}
var _ resource.ResourceWithImportState = &flyVolumeResource{}

type flyVolumeResource struct {
	state *providerstate.State
}

func NewVolumeResource() resource.Resource {
	return &flyVolumeResource{}
}

func (r *flyVolumeResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "fly_volume"
}

func (r *flyVolumeResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.state = req.ProviderData.(*providerstate.State)
}

type flyVolumeResourceData struct {
	Id        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Size      types.Int64  `tfsdk:"size"`
	App       types.String `tfsdk:"app"`
	Region    types.String `tfsdk:"region"`
	Encrypted types.Bool   `tfsdk:"encrypted"`
}

func (r *flyVolumeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: ID_DESC,
				Computed:            true,
				// Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"app": schema.StringAttribute{
				MarkdownDescription: APP_DESC,
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"size": schema.Int64Attribute{
				MarkdownDescription: "Size of volume in GB",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: NAME_DESC,
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: REGION_DESC,
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"encrypted": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
					boolplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *flyVolumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data flyVolumeResourceData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	machineApi := machineapi.NewMachineApi(ctx, r.state)
	q, err := machineApi.CreateVolume(ctx, data.Name.ValueString(), data.App.ValueString(), data.Region.ValueString(), int(data.Size.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create volume", err.Error())
		tflog.Warn(ctx, fmt.Sprintf("%+v", err))
		return
	}

	data = flyVolumeResourceData{
		Id:        types.StringValue(q.ID),
		Name:      types.StringValue(q.Name),
		Size:      types.Int64Value(int64(q.SizeGb)),
		App:       types.StringValue(data.App.ValueString()),
		Region:    types.StringValue(q.Region),
		Encrypted: types.BoolValue(q.Encrypted),
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *flyVolumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data flyVolumeResourceData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.Id.ValueString()

	if id == "" {
		resp.Diagnostics.AddError("Failed to read volume", "id is empty")
		return
	}
	app := data.App.ValueString()

	machineApi := machineapi.NewMachineApi(ctx, r.state)
	query, err := machineApi.GetVolume(ctx, id, app)
	if err != nil {
		resp.Diagnostics.AddError("Query failed", err.Error())
		return
	}

	data.Name = types.StringValue(query.Name)
	data.Size = types.Int64Value(int64(query.SizeGb))
	data.Encrypted = types.BoolValue(query.Encrypted)

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *flyVolumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var current flyVolumeResourceData
	diags := req.State.Get(ctx, &current)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var new flyVolumeResourceData
	diags = req.Plan.Get(ctx, &new)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	app := current.App.ValueString()
	id := current.Id.ValueString()

	newSize := new.Size.ValueInt64()
	if newSize != current.Size.ValueInt64() {
		machineApi := machineapi.NewMachineApi(ctx, r.state)
		err := machineApi.ExtendVolume(ctx, app, id, int(newSize))
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Extending app [%s] volume [%s] failed", app, id), err.Error())
			return
		}
		current.Size = new.Size
	}

	diags = resp.State.Set(ctx, current)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *flyVolumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data flyVolumeResourceData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Id.IsUnknown() && !data.Id.IsNull() && data.Id.ValueString() != "" {
		machineApi := machineapi.NewMachineApi(ctx, r.state)
		err := machineApi.DeleteVolume(ctx, data.App.ValueString(), data.Id.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Delete volume failed", err.Error())
			return
		}
	}

	resp.State.RemoveResource(ctx)
}

func (vr flyVolumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, ",")

	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: app_id,volume_id. Got: %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), idParts[1])...)
}
