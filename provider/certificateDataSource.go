package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/zakhar-gulchak/terraform-provider-fly/graphql"
	"github.com/zakhar-gulchak/terraform-provider-fly/providerstate"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &certDataSourceType{}
var _ datasource.DataSourceWithConfigure = &certDataSourceType{}

type certDataSourceType struct {
	state *providerstate.State
}

func (d *certDataSourceType) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "fly_cert"
}

// Matches Schema
type certDataSourceOutput struct {
	Id                        types.String `tfsdk:"id"`
	Appid                     types.String `tfsdk:"app"`
	DnsValidationInstructions types.String `tfsdk:"dns_validation_instructions"`
	DnsValidationHostname     types.String `tfsdk:"dns_validation_hostname"`
	DnsValidationTarget       types.String `tfsdk:"dns_validation_target"`
	Hostname                  types.String `tfsdk:"hostname"`
	Check                     types.Bool   `tfsdk:"check"`
}

func (d *certDataSourceType) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"app": schema.StringAttribute{
				MarkdownDescription: APP_DESC,
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: ID_DESC,
				Computed:            true,
			},
			"dns_validation_instructions": schema.StringAttribute{
				Computed: true,
			},
			"dns_validation_target": schema.StringAttribute{
				Computed: true,
			},
			"dns_validation_hostname": schema.StringAttribute{
				Computed: true,
			},
			"check": schema.BoolAttribute{
				Computed: true,
			},
			"hostname": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

func NewCertDataSource() datasource.DataSource {
	return &certDataSourceType{}
}

func (d *certDataSourceType) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.state = req.ProviderData.(*providerstate.State)
}

func (d *certDataSourceType) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data certDataSourceOutput

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	hostname := data.Hostname.ValueString()
	app := data.Appid.ValueString()

	query, err := graphql.GetCertificate(ctx, d.state.GraphqlClient, app, hostname)
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, err := range errList {
			if err.Message == "Could not resolve " {
				return
			}
			resp.Diagnostics.AddError(err.Message, err.Path.String())
		}
	} else if err != nil {
		resp.Diagnostics.AddError("Read: query failed", err.Error())
		return
	}

	data = certDataSourceOutput{
		Id:                        types.StringValue(query.App.Certificate.Id),
		Appid:                     data.Appid,
		DnsValidationInstructions: types.StringValue(query.App.Certificate.DnsValidationInstructions),
		DnsValidationHostname:     types.StringValue(query.App.Certificate.DnsValidationHostname),
		DnsValidationTarget:       types.StringValue(query.App.Certificate.DnsValidationTarget),
		Hostname:                  types.StringValue(query.App.Certificate.Hostname),
		Check:                     types.BoolValue(query.App.Certificate.Check),
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}
