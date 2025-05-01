package provider

import (
	"context"

	"github.com/zakhar-gulchak/terraform-provider-fly/machineapi"
	"github.com/zakhar-gulchak/terraform-provider-fly/providerstate"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &volumeDataSourceType{}
var _ datasource.DataSourceWithConfigure = &appDataSourceType{}

type volumeDataSourceType struct {
	state *providerstate.State
}

func NewVolumeDataSource() datasource.DataSource {
	return &volumeDataSourceType{}
}

func (d *volumeDataSourceType) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "fly_volume"
}

func (d *volumeDataSourceType) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.state = req.ProviderData.(*providerstate.State)
}

// Matches Schema
type volumeDataSourceOutput struct {
	Id        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Size      types.Int64  `tfsdk:"size"`
	App       types.String `tfsdk:"app"`
	Region    types.String `tfsdk:"region"`
	Encrypted types.Bool   `tfsdk:"encrypted"`
}

func (d *volumeDataSourceType) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: ID_DESC,
				Required:            true,
			},
			"app": schema.StringAttribute{
				MarkdownDescription: APP_DESC,
				Required:            true,
			},
			"size": schema.Int64Attribute{
				MarkdownDescription: "Size of volume in GB",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: NAME_DESC,
				Computed:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: REGION_DESC,
				Computed:            true,
			},
			"encrypted": schema.BoolAttribute{
				Computed: true,
			},
		},
	}
}

func (d *volumeDataSourceType) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data volumeDataSourceOutput
	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.Id.ValueString()
	app := data.App.ValueString()

	machineApi := machineapi.NewMachineApi(ctx, d.state)
	query, err := machineApi.GetVolume(ctx, id, app)
	if err != nil {
		resp.Diagnostics.AddError("Query failed", err.Error())
		return
	}

	data = volumeDataSourceOutput{
		Id:        types.StringValue(query.ID),
		Name:      types.StringValue(query.Name),
		Size:      types.Int64Value(int64(query.SizeGb)),
		App:       types.StringValue(data.App.ValueString()),
		Region:    types.StringValue(query.Region),
		Encrypted: types.BoolValue(query.Encrypted),
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
