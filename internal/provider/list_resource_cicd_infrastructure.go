package provider

import (
	"context"
	"iter"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// CICDInfrastructureListModel describes the configuration of the
// defectdojo_cicd_infrastructure list resource.
type CICDInfrastructureListModel struct {
	InfrastructureType types.String `tfsdk:"infrastructure_type"`
	NameContains       types.String `tfsdk:"name_contains"`
}

func NewCICDInfrastructureListResource() list.ListResource {
	id := func(ci *defectdojo.CICDInfrastructure) int { return ci.ID }
	return &listResource[CICDInfrastructureListModel, defectdojo.CICDInfrastructure]{
		typeName: "_cicd_infrastructure",
		schema: schema.Schema{
			MarkdownDescription: "Lists the DefectDojo CI/CD infrastructure objects, ordered by ID. " +
				"`description` and `url` are only read for users with the view or change permission." + listNotes,
			Attributes: map[string]schema.Attribute{
				"infrastructure_type": schema.StringAttribute{
					MarkdownDescription: "Only list the objects of this type: `scm_server`, `build_server` or `orchestration`.",
					Optional:            true,
					// DefectDojo answers another value with a 400.
					Validators: []validator.String{stringvalidator.OneOf(cicdInfrastructureTypes...)},
				},
				"name_contains": listContainsAttribute("objects", "name"),
			},
		},
		objects: "CI/CD infrastructure",
		all: func(ctx context.Context, client *defectdojo.Client, config CICDInfrastructureListModel) iter.Seq2[*defectdojo.CICDInfrastructure, error] {
			return client.CICDInfrastructure.ListAll(ctx, &defectdojo.ListCICDInfrastructureOptions{
				InfrastructureType: defectdojo.CICDInfrastructureType(config.InfrastructureType.ValueString()),
			})
		},
		id: id,
		// DefectDojo's name filter is exact, so names are matched here.
		keep: func(ci *defectdojo.CICDInfrastructure, config CICDInfrastructureListModel) bool {
			return containsFold(ci.Name, config.NameContains)
		},
		results: modelResults[CICDInfrastructureListModel](id,
			func(ci *defectdojo.CICDInfrastructure) string { return ci.Name },
			updateCICDInfrastructureModel),
	}
}
