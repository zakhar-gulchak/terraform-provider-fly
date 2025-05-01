package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zakhar-gulchak/terraform-provider-fly/graphql"
	"github.com/zakhar-gulchak/terraform-provider-fly/providerstate"
	"github.com/zakhar-gulchak/terraform-provider-fly/utils"
)

var _ resource.Resource = &flyAppResource{}
var _ resource.ResourceWithConfigure = &flyAppResource{}
var _ resource.ResourceWithImportState = &flyAppResource{}

type flyAppResource struct {
	state *providerstate.State
}

func NewAppResource() resource.Resource {
	return &flyAppResource{}
}

func (r *flyAppResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "fly_app"
}

func (r *flyAppResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.state = req.ProviderData.(*providerstate.State)
}

type flyAppResourceData struct {
	Name                  types.String `tfsdk:"name"`
	Org                   types.String `tfsdk:"org"`
	OrgId                 types.String `tfsdk:"org_id"`
	AppUrl                types.String `tfsdk:"app_url"`
	Id                    types.String `tfsdk:"id"`
	AssignSharedIpAddress types.Bool   `tfsdk:"assign_shared_ip_address"`
	SharedIpAddress       types.String `tfsdk:"shared_ip_address"`
}

func (r *flyAppResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			// Key
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of application",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"org": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "The name of the organization to generate the app in, ex: `personal` (your initial org)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			// RW
			"assign_shared_ip_address": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Assign a shared ipv4 address to the app. Note that depending on conditions an app may get a shared ip automatically.",
			},

			// RO
			"org_id": schema.StringAttribute{
				Computed: true,
			},
			"id": schema.StringAttribute{
				Computed: true,
			},
			"app_url": schema.StringAttribute{
				Computed: true,
			},
			"shared_ip_address": schema.StringAttribute{
				MarkdownDescription: SHAREDIP_DESC,
				Computed:            true,
			},
		},
	}
}

func (r *flyAppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data flyAppResourceData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Org.IsUnknown() {
		defaultOrg, err := utils.GetDefaultOrg(ctx, r.state.GraphqlClient)
		if err != nil {
			resp.Diagnostics.AddError("Could not detect default organization", err.Error())
			return
		}
		data.OrgId = types.StringValue(defaultOrg.Id)
		data.Org = types.StringValue(defaultOrg.Name)
	} else {
		org, err := graphql.Organization(ctx, r.state.GraphqlClient, data.Org.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Could not resolve organization", err.Error())
			return
		}
		data.OrgId = types.StringValue(org.Organization.Id)
	}
	name := data.Name.ValueString()
	org := data.OrgId.ValueString()
	mresp, err := graphql.CreateAppMutation(ctx, r.state.GraphqlClient, name, org)
	if err != nil {
		utils.HandleGraphqlErrors(&resp.Diagnostics, err, "Error creating app (name [%s], org %s)", name, org)
		return
	}
	data.OrgId = types.StringValue(mresp.CreateApp.App.Organization.Id)
	data.AppUrl = types.StringValue(mresp.CreateApp.App.AppUrl)
	data.Id = types.StringValue(mresp.CreateApp.App.Id)

	err = utils.Retry(ctx, time.Minute*5, time.Second*5, func() (temporaryErr error, finalErr error) {
		name := data.Name.ValueString()
		_, err := graphql.GetFullApp(ctx, r.state.GraphqlClient, name)
		if err != nil {
			return err, nil
		}
		return nil, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Consistency error; upstream API never successfully responded to request for newly created app", err.Error())
		return
	}

	if data.AssignSharedIpAddress.ValueBool() {
		mresp2, err := graphql.AllocateIpAddress(ctx, r.state.GraphqlClient, name, "global", "shared_v4")
		if err != nil {
			utils.HandleGraphqlErrors(&resp.Diagnostics, err, "Error allocating shared ipv4 address (app [%s])", name)
			return
		}
		data.SharedIpAddress = types.StringValue(mresp2.AllocateIpAddress.App.SharedIpAddress)
	} else {
		data.SharedIpAddress = types.StringValue(mresp.CreateApp.App.SharedIpAddress)
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *flyAppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data flyAppResourceData
	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	query, err := graphql.GetFullApp(ctx, r.state.GraphqlClient, name)
	if err != nil {
		utils.HandleGraphqlErrors(&resp.Diagnostics, err, "Error looking up app (name [%s])", name)
		return
	}

	data.Org = types.StringValue(query.App.Organization.Slug)
	data.OrgId = types.StringValue(query.App.Organization.Id)
	data.AppUrl = types.StringValue(query.App.AppUrl)
	data.Id = types.StringValue(query.App.Id)
	data.SharedIpAddress = types.StringValue(query.App.SharedIpAddress)

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *flyAppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan flyAppResourceData
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var data flyAppResourceData
	diags = resp.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	enableSharedIp := plan.AssignSharedIpAddress.ValueBool()
	if !plan.AssignSharedIpAddress.IsNull() && enableSharedIp != data.AssignSharedIpAddress.ValueBool() {
		name := plan.Name.ValueString()
		if enableSharedIp {
			mresp2, err := graphql.AllocateIpAddress(ctx, r.state.GraphqlClient, name, "global", "shared_v4")
			if err != nil {
				utils.HandleGraphqlErrors(&resp.Diagnostics, err, "Error allocating shared ipv4 address (app [%s])", name)
				return
			}
			data.SharedIpAddress = types.StringValue(mresp2.AllocateIpAddress.App.SharedIpAddress)
		} else {
			_, err := graphql.ReleaseIpAddress(ctx, r.state.GraphqlClient, plan.Name.ValueString(), data.SharedIpAddress.ValueString())
			if err != nil {
				utils.HandleGraphqlErrors(&resp.Diagnostics, err, "Error deleting shared ip address (app [%s], addr [%s])", plan.Name.ValueString(), plan.SharedIpAddress.ValueString())
				return
			}
			data.SharedIpAddress = types.StringValue("")
		}
	}

	if !plan.AssignSharedIpAddress.IsNull() {
		data.AssignSharedIpAddress = types.BoolValue(enableSharedIp)
	}

	diags = resp.State.Set(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyAppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data flyAppResourceData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	name := data.Name.ValueString()
	_, err := graphql.DeleteAppMutation(ctx, r.state.GraphqlClient, name)
	if err != nil {
		utils.HandleGraphqlErrors(&resp.Diagnostics, err, "Error deleting app (name [%s])", name)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r flyAppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
