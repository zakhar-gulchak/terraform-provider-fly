package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zakhar-gulchak/terraform-provider-fly/graphql"
	"github.com/zakhar-gulchak/terraform-provider-fly/providerstate"
	"github.com/zakhar-gulchak/terraform-provider-fly/utils"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &appDataSourceType{}
var _ datasource.DataSourceWithConfigure = &appDataSourceType{}

func NewAppDataSource() datasource.DataSource {
	return &appDataSourceType{}
}

type appDataSourceType struct {
	state *providerstate.State
}

func (d *appDataSourceType) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "fly_app"
}

func (d *appDataSourceType) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.state = req.ProviderData.(*providerstate.State)
}

type appDataSourceOutput struct {
	Name            types.String `tfsdk:"name"`
	AppUrl          types.String `tfsdk:"appurl"`
	Hostname        types.String `tfsdk:"hostname"`
	Id              types.String `tfsdk:"id"`
	Status          types.String `tfsdk:"status"`
	Deployed        types.Bool   `tfsdk:"deployed"`
	Healthchecks    []string     `tfsdk:"healthchecks"`
	Ipaddresses     []string     `tfsdk:"ipaddresses"`
	Sharedipaddress types.String `tfsdk:"sharedipaddress"`
	Currentrelease  types.String `tfsdk:"currentrelease"`
}

func (d *appDataSourceType) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of app",
				Required:            true,
			},
			"appurl": schema.StringAttribute{
				Computed: true,
			},
			"hostname": schema.StringAttribute{
				Computed: true,
			},
			"id": schema.StringAttribute{
				Computed: true,
			},
			"status": schema.StringAttribute{
				Computed: true,
			},
			"deployed": schema.BoolAttribute{
				Computed: true,
			},
			"healthchecks": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
			"ipaddresses": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
			"sharedipaddress": schema.StringAttribute{
				MarkdownDescription: SHAREDIP_DESC,
				Computed:            true,
			},
			"currentrelease": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *appDataSourceType) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data appDataSourceOutput

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	appName := data.Name.ValueString()

	queryresp, err := graphql.GetFullApp(ctx, d.state.GraphqlClient, appName)
	if err != nil {
		utils.HandleGraphqlErrors(&resp.Diagnostics, err, "Error looking up app (name [%s])", appName)
		return
	}

	a := appDataSourceOutput{
		Name:            data.Name,
		AppUrl:          types.StringValue(queryresp.App.AppUrl),
		Hostname:        types.StringValue(queryresp.App.Hostname),
		Id:              types.StringValue(queryresp.App.Id),
		Status:          types.StringValue(queryresp.App.Status),
		Deployed:        types.BoolValue(queryresp.App.Deployed),
		Sharedipaddress: types.StringValue(queryresp.App.SharedIpAddress),
		Currentrelease:  types.StringValue(queryresp.App.CurrentRelease.Id),
	}

	for _, s := range queryresp.App.HealthChecks.Nodes {
		a.Healthchecks = append(a.Healthchecks, s.Name+": "+s.Status)
	}

	for _, s := range queryresp.App.IpAddresses.Nodes {
		a.Ipaddresses = append(a.Ipaddresses, s.Address)
	}

	diags = resp.State.Set(ctx, &a)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
