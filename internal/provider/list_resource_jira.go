package provider

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// JiraInstanceListModel describes the configuration of the
// defectdojo_jira_instance list resource.
type JiraInstanceListModel struct {
	ConfigurationNameContains types.String `tfsdk:"configuration_name_contains"`
}

func NewJiraInstanceListResource() list.ListResource {
	id := func(ji *defectdojo.JiraInstance) int { return ji.ID }
	return &listResource[JiraInstanceListModel, defectdojo.JiraInstance]{
		typeName: "_jira_instance",
		schema: schema.Schema{
			MarkdownDescription: "Lists the DefectDojo Jira instances, ordered by ID, with their configuration name, or their URL when it is empty, as display name. " +
				"The generated configuration has no `password_wo`, which DefectDojo never returns: add it to change the URL, and with a new `password_wo_version` to change the password." + listNotes,
			Attributes: map[string]schema.Attribute{
				"configuration_name_contains": listContainsAttribute("Jira instances", "configuration name"),
			},
		},
		objects: "Jira instances",
		all: func(ctx context.Context, client *defectdojo.Client, _ JiraInstanceListModel) iter.Seq2[*defectdojo.JiraInstance, error] {
			return client.JiraInstances.ListAll(ctx, nil)
		},
		id: id,
		keep: func(ji *defectdojo.JiraInstance, config JiraInstanceListModel) bool {
			return containsFold(ji.ConfigurationName, config.ConfigurationNameContains)
		},
		results: modelResults[JiraInstanceListModel](id,
			func(ji *defectdojo.JiraInstance) string {
				if ji.ConfigurationName == "" {
					return ji.URL
				}
				return ji.ConfigurationName
			},
			updateJiraInstanceModel),
	}
}

// JiraProjectListModel describes the configuration of the
// defectdojo_jira_project list resource.
type JiraProjectListModel struct {
	ProductID    types.Int64 `tfsdk:"product_id"`
	EngagementID types.Int64 `tfsdk:"engagement_id"`
}

func NewJiraProjectListResource() list.ListResource {
	return &listResource[JiraProjectListModel, defectdojo.JiraProject]{
		typeName: "_jira_project",
		schema: schema.Schema{
			MarkdownDescription: "Lists the DefectDojo Jira projects, the Jira configurations of products and engagements, ordered by ID, " +
				"with the display name `<project key> / <product or engagement>`." + listNotes,
			Attributes: map[string]schema.Attribute{
				"product_id":    listIDAttribute("Only list the Jira project of the product with this ID."),
				"engagement_id": listIDAttribute("Only list the Jira project of the engagement with this ID."),
			},
		},
		objects: "Jira projects",
		all: func(ctx context.Context, client *defectdojo.Client, config JiraProjectListModel) iter.Seq2[*defectdojo.JiraProject, error] {
			return client.JiraProjects.ListAll(ctx, &defectdojo.ListJiraProjectsOptions{
				Product:    int(config.ProductID.ValueInt64()),
				Engagement: int(config.EngagementID.ValueInt64()),
			})
		},
		id:      func(p *defectdojo.JiraProject) int { return p.ID },
		results: jiraProjectResults,
		failure: func(err error, config JiraProjectListModel) diag.Diagnostics {
			// The product and engagement filters reject objects that don't
			// exist with a 400.
			var errResp *defectdojo.ErrorResponse
			if errors.As(err, &errResp) && errResp.StatusCode == http.StatusBadRequest {
				var diags diag.Diagnostics
				switch {
				case len(errResp.Fields["product"]) > 0:
					diags.AddError("DefectDojo product not found",
						fmt.Sprintf("There is no product with the ID %d, or the provider user may not see it.", config.ProductID.ValueInt64()))
					return diags
				case len(errResp.Fields["engagement"]) > 0:
					diags.AddError("DefectDojo engagement not found",
						fmt.Sprintf("There is no engagement with the ID %d, or the provider user may not see it.", config.EngagementID.ValueInt64()))
					return diags
				}
			}
			return listError("Jira projects")(err)
		},
	}
}

// jiraProjectResults builds the results of Jira projects, whose display name
// names their product or engagement; each is read once.
func jiraProjectResults(ctx context.Context, client *defectdojo.Client, req list.ListRequest, _ JiraProjectListModel) func(*defectdojo.JiraProject) list.ListResult {
	names := map[string]string{}
	name := func(p *defectdojo.JiraProject) string {
		key := fmt.Sprintf("product %d", p.Product)
		if p.Engagement != 0 {
			key = fmt.Sprintf("engagement %d", p.Engagement)
		}
		if n, ok := names[key]; ok {
			return n
		}
		// The ID stands in for an object that can't be read.
		names[key] = key
		if p.Engagement != 0 {
			if e, _, err := client.Engagements.Get(ctx, p.Engagement, nil); err == nil {
				names[key] = e.Name
			}
		} else if pr, _, err := client.Products.Get(ctx, p.Product, nil); err == nil {
			names[key] = pr.Name
		}
		return names[key]
	}
	return func(p *defectdojo.JiraProject) list.ListResult {
		var state JiraProjectResourceModel
		updateJiraProjectModel(&state, p)
		return newListResult(ctx, req, p.ProjectKey+" / "+name(p), p.ID, state)
	}
}
