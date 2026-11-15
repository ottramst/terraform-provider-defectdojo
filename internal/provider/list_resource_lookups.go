package provider

import (
	"context"
	"iter"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/ottramst/go-defectdojo"
)

func NewSLAConfigurationListResource() list.ListResource {
	return nameListResource("_sla_configuration", "SLA configurations",
		"Lists the DefectDojo SLA configurations, ordered by ID, including the Default one (ID 1).",
		func(ctx context.Context, client *defectdojo.Client) iter.Seq2[*defectdojo.SLAConfiguration, error] {
			return client.SLAConfigurations.ListAll(ctx, nil)
		},
		func(sla *defectdojo.SLAConfiguration) int { return sla.ID },
		func(sla *defectdojo.SLAConfiguration) string { return sla.Name },
		updateSLAConfigurationModel)
}

func NewNotificationWebhookListResource() list.ListResource {
	id := func(wh *defectdojo.NotificationWebhook) int { return wh.ID }
	return nameListResource("_notification_webhook", "notification webhooks",
		"Lists the DefectDojo notification webhooks, ordered by ID. "+
			"The generated configuration has no `header_value_wo`, which DefectDojo returns in plain text: add it, with a `header_value_wo_version`, to manage it.",
		// DefectDojo returns webhooks in no particular order, so paging could
		// skip some: they are read first, and sorted.
		func(ctx context.Context, client *defectdojo.Client) iter.Seq2[*defectdojo.NotificationWebhook, error] {
			return sortedByID(client.NotificationWebhooks.ListAll(ctx, nil), id)
		},
		id,
		func(wh *defectdojo.NotificationWebhook) string { return wh.Name },
		updateNotificationWebhookModel)
}

func NewRegulationListResource() list.ListResource {
	return nameListResource("_regulation", "regulations",
		"Lists the DefectDojo regulations, ordered by ID, including those DefectDojo comes with.",
		func(ctx context.Context, client *defectdojo.Client) iter.Seq2[*defectdojo.Regulation, error] {
			return client.Regulations.ListAll(ctx, nil)
		},
		func(reg *defectdojo.Regulation) int { return reg.ID },
		func(reg *defectdojo.Regulation) string { return reg.Name },
		updateRegulationModel)
}

func NewNoteTypeListResource() list.ListResource {
	return nameListResource("_note_type", "note types",
		"Lists the DefectDojo note types, ordered by ID.",
		func(ctx context.Context, client *defectdojo.Client) iter.Seq2[*defectdojo.NoteType, error] {
			return client.NoteTypes.ListAll(ctx, nil)
		},
		func(nt *defectdojo.NoteType) int { return nt.ID },
		func(nt *defectdojo.NoteType) string { return nt.Name },
		updateNoteTypeModel)
}

func NewDevelopmentEnvironmentListResource() list.ListResource {
	return nameListResource("_development_environment", "development environments",
		"Lists the DefectDojo development environments, ordered by ID, including those DefectDojo comes with.",
		func(ctx context.Context, client *defectdojo.Client) iter.Seq2[*defectdojo.DevelopmentEnvironment, error] {
			return client.DevelopmentEnvironments.ListAll(ctx, nil)
		},
		func(env *defectdojo.DevelopmentEnvironment) int { return env.ID },
		func(env *defectdojo.DevelopmentEnvironment) string { return env.Name },
		updateDevelopmentEnvironmentModel)
}

func NewProductPlatformListResource() list.ListResource {
	return productAttributeListResource(productPlatformKind)
}

func NewProductLifecycleListResource() list.ListResource {
	return productAttributeListResource(productLifecycleKind)
}

func NewProductOriginListResource() list.ListResource {
	return productAttributeListResource(productOriginKind)
}

// productAttributeListResource returns the list resource of a kind of
// product attribute, whose display name is its value.
func productAttributeListResource(kind productAttributeKind) list.ListResource {
	id := func(a *defectdojo.ProductAttribute) int { return a.ID }
	r := nameListResource("_"+kind.typeName, kind.object+"s",
		"Lists the DefectDojo "+kind.object+"s, ordered by ID, including those DefectDojo comes with, with their `value` as display name.",
		// DefectDojo orders them by display order and name, which needn't be
		// unique, so paging could skip some: they are read first, and sorted.
		func(ctx context.Context, client *defectdojo.Client) iter.Seq2[*defectdojo.ProductAttribute, error] {
			return sortedByID(kind.service(client).ListAll(ctx, nil), id)
		},
		id,
		func(a *defectdojo.ProductAttribute) string { return a.Name },
		updateProductAttributeModel)
	// Products show the value, so the filter matches it too.
	r.schema.Attributes["name_contains"] = listContainsAttribute(kind.object+"s", "name or value")
	r.keep = func(a *defectdojo.ProductAttribute, config NameListModel) bool {
		return containsFold(a.Name, config.NameContains) || containsFold(a.Value, config.NameContains)
	}
	r.results = modelResults[NameListModel](id, func(a *defectdojo.ProductAttribute) string { return a.Value }, updateProductAttributeModel)
	return r
}
