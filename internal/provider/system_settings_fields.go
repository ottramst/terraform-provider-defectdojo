package provider

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// The system settings are generated from one table, systemSettingsFields,
// rather than written out one by one: there are about 70. Their values move
// between Terraform and the client through the client's JSON field names: a
// read marshals the client's SystemSettings to a map, and a write builds a
// map of the configured settings and unmarshals it into a
// SystemSettingsPatch. TestSystemSettingsFields checks that every name is a
// field of both.
//
// Unlike the other resources, text settings keep DefectDojo's value as it is,
// "" included, rather than mapping "" to null: a null configuration means
// "not managed", so "" is how a configuration clears a setting.

// settingKind is the kind of value of a system setting.
type settingKind int

const (
	settingBool settingKind = iota
	settingInt
	// settingNullableInt is an int that DefectDojo can also hold as null,
	// which Terraform can't set: null means the setting isn't managed.
	settingNullableInt
	// settingText is text; "" clears it. DefectDojo reads a null as "".
	settingText
)

// systemSetting describes a system setting.
type systemSetting struct {
	name string // the attribute name
	json string // the JSON name, when it differs from the attribute name
	kind settingKind

	// maxLength is the maximum length of text, in characters.
	maxLength int

	description string

	stringValidators []validator.String
	int64Validators  []validator.Int64
}

func (s systemSetting) jsonName() string {
	if s.json != "" {
		return s.json
	}
	return s.name
}

var (
	passwordLengthValidators = []validator.Int64{int64validator.Between(9, 48)}
	jiraSeverities           = []string{"Critical", "High", "Medium", "Low", "Info", ""}
)

// systemSettingsFields are the system settings that the resource and the data
// source manage, in the order of DefectDojo's schema. read_only_mode is left
// out on purpose, and the secrets have write-only attributes instead.
var systemSettingsFields = []systemSetting{
	{name: "enable_deduplication", kind: settingBool, description: "Whether DefectDojo marks new findings that match an existing finding (by endpoints, CWE and title) as duplicates."},
	{name: "delete_duplicates", kind: settingBool, description: "Whether DefectDojo deletes the oldest duplicates of a finding beyond `max_dupes`."},
	{name: "max_dupes", kind: settingNullableInt, description: "The number of duplicates of a finding to keep when `delete_duplicates` is on; `0` deletes them all. DefectDojo can also have no limit (null), which only its UI can set."},
	{name: "email_from", kind: settingText, maxLength: 200, description: "The sender address of notification mails."},
	{name: "enable_jira", kind: settingBool, description: "Whether the Jira integration is on. It is needed to push findings to Jira, not to manage `defectdojo_jira_instance` and `defectdojo_jira_project`. DefectDojo also lists the configuration permissions of Jira instances only while it is on."},
	{name: "enable_jira_web_hook", kind: settingBool, description: "Whether DefectDojo accepts Jira's webhook calls, which need the Jira webhook secret (`jira_webhook_secret_wo`) in their URL unless `disable_jira_webhook_secret` is on."},
	{name: "disable_jira_webhook_secret", kind: settingBool, description: "Whether Jira's webhook calls are accepted without the secret, which DefectDojo discourages."},
	{name: "jira_minimum_severity", kind: settingText, description: "The lowest severity of findings that are pushed to Jira: `Critical`, `High`, `Medium`, `Low`, `Info`, or `\"\"` for all.",
		stringValidators: []validator.String{stringvalidator.OneOf(jiraSeverities...)}},
	{name: "jira_labels", kind: settingText, maxLength: 200, description: "The labels of new Jira issues, separated by spaces."},
	{name: "add_vulnerability_id_to_jira_label", kind: settingBool, description: "Whether a finding's vulnerability IDs are added to the labels of its Jira issue."},
	{name: "enable_github", kind: settingBool, description: "Whether the GitHub integration is on."},
	{name: "enable_slack_notifications", kind: settingBool, description: "Whether notifications can go to Slack (the `slack` channel of `defectdojo_notifications`). It needs `slack_token_wo`."},
	{name: "slack_channel", kind: settingText, maxLength: 100, description: "The Slack channel of system-wide notifications."},
	{name: "slack_username", kind: settingText, maxLength: 100, description: "The name DefectDojo posts to Slack as, or `\"\"` for the bot's name."},
	{name: "enable_msteams_notifications", kind: settingBool, description: "Whether notifications can go to Microsoft Teams (the `msteams` channel of `defectdojo_notifications`). It needs `msteams_url_wo`."},
	{name: "enable_mail_notifications", kind: settingBool, description: "Whether notifications can be mailed (the `mail` channel of `defectdojo_notifications`)."},
	{name: "mail_notifications_to", kind: settingText, maxLength: 200, description: "The address of system-wide notification mails."},
	{name: "enable_webhooks_notifications", kind: settingBool, description: "Whether notifications can go to webhooks (the `webhooks` channel of `defectdojo_notifications`; see `defectdojo_notification_webhook`)."},
	{name: "webhooks_notifications_timeout", kind: settingInt, description: "How many seconds DefectDojo waits for a webhook to answer."},
	{name: "enforce_verified_status", kind: settingBool, description: "Whether grading, Jira, metrics and reports consider only verified findings. It overrides the three `enforce_verified_status_*` settings."},
	{name: "enforce_verified_status_jira", kind: settingBool, description: "Whether only verified findings are pushed to Jira."},
	{name: "enforce_verified_status_product_grading", kind: settingBool, description: "Whether only verified findings count for a product's grade."},
	{name: "enforce_verified_status_metrics", kind: settingBool, description: "Whether only verified findings count in metrics, reports and filters."},
	{name: "false_positive_history", kind: settingBool, description: "Whether DefectDojo marks a new finding as a false positive when an equal finding of the product was (experimental; doesn't work with deduplication)."},
	{name: "retroactive_false_positive_history", kind: settingBool, description: "Whether the false positive history also marks or unmarks the existing equal findings of the product (experimental; needs `false_positive_history`)."},
	{name: "url_prefix", kind: settingText, maxLength: 300, description: "The path DefectDojo is served under, when it isn't served at the root."},
	{name: "team_name", kind: settingText, maxLength: 100, description: "The name of the team."},
	{name: "enable_product_grade", kind: settingBool, description: "Whether products show a grade letter for their overall health."},
	{name: "product_grade_a", kind: settingInt, description: "The percentage score a product needs for an A."},
	{name: "product_grade_b", kind: settingInt, description: "The percentage score a product needs for a B."},
	{name: "product_grade_c", kind: settingInt, description: "The percentage score a product needs for a C."},
	{name: "product_grade_d", kind: settingInt, description: "The percentage score a product needs for a D."},
	{name: "product_grade_f", kind: settingInt, description: "The percentage score at or below which a product gets an F."},
	{name: "enable_product_tag_inheritance", kind: settingBool, description: "Whether every product passes its tags on to its engagements, tests and findings."},
	{name: "enable_benchmark", kind: settingBool, description: "Whether benchmarks, such as the OWASP ASVS, are on."},
	{name: "enable_similar_findings", kind: settingBool, description: "Whether a finding's page lists similar findings, which can be slow."},
	{name: "engagement_auto_close", kind: settingBool, description: "Whether DefectDojo closes engagements `engagement_auto_close_days` days after their end date."},
	{name: "engagement_auto_close_days", kind: settingInt, description: "The number of days after their end date that engagements are closed when `engagement_auto_close` is on."},
	{name: "enable_finding_sla", kind: settingBool, description: "Whether findings get SLA expiration dates. **While it is off, DefectDojo never finishes recalculating SLA dates**, so an SLA configuration whose days changed, and the products using it, stay locked (see `defectdojo_sla_configuration`)."},
	{name: "enable_notify_sla_active", kind: settingBool, description: "Whether DefectDojo notifies when active findings breach their SLA."},
	{name: "enable_notify_sla_active_verified", kind: settingBool, description: "Whether DefectDojo notifies when active, verified findings breach their SLA."},
	{name: "enable_notify_sla_jira_only", kind: settingBool, description: "Whether SLA breach notifications are sent only for findings with a Jira issue."},
	{name: "enable_notify_sla_exponential_backoff", kind: settingBool, description: "Whether SLA breach notifications back off exponentially (after 1, 2, 4, 8 days, …) rather than coming every day."},
	{name: "allow_anonymous_survey_response", json: "allow_anonymous_survey_repsonse", kind: settingBool, description: "Whether anyone with a survey's link can answer it."}, //nolint:misspell // DefectDojo's JSON name.
	{name: "disclaimer_notifications", kind: settingText, maxLength: 3000, description: "A disclaimer added to every notification."},
	{name: "disclaimer_reports", kind: settingText, maxLength: 5000, description: "A disclaimer added to reports."},
	{name: "disclaimer_reports_forced", kind: settingBool, description: "Whether the report disclaimer is added to every report, even when it isn't chosen."},
	{name: "disclaimer_notes", kind: settingText, maxLength: 3000, description: "A disclaimer shown next to the form for notes."},
	{name: "risk_acceptance_form_default_days", kind: settingNullableInt, description: "The default number of days until a risk acceptance expires. DefectDojo can also have none (null), which only its UI can set."},
	{name: "risk_acceptance_notify_before_expiration", kind: settingNullableInt, description: "How many days before a risk acceptance expires DefectDojo notifies. DefectDojo can also not notify (null), which only its UI can set."},
	{name: "enable_questionnaires", kind: settingBool, description: "Whether questionnaires are on. DefectDojo also lists the configuration permissions of questionnaires only while it is on."},
	{name: "enable_checklists", kind: settingBool, description: "Whether checklists are on."},
	{name: "enable_endpoint_metadata_import", kind: settingBool, description: "Whether the import of endpoint metadata is on."},
	{name: "enable_user_profile_editable", kind: settingBool, description: "Whether users can edit their profiles."},
	{name: "enable_product_tracking_files", kind: settingBool, description: "Whether product tracking files are on."},
	{name: "enable_finding_groups", kind: settingBool, description: "Whether finding groups are on."},
	{name: "enable_ui_table_based_searching", kind: settingBool, description: "Whether table headings also sort the current page."},
	{name: "enable_calendar", kind: settingBool, description: "Whether the calendar is on."},
	{name: "enable_cvss3_display", kind: settingBool, description: "Whether the UI shows CVSS 3 fields."},
	{name: "enable_cvss4_display", kind: settingBool, description: "Whether the UI shows CVSS 4 fields."},
	{name: "minimum_password_length", kind: settingInt, int64Validators: passwordLengthValidators, description: "The minimum length of users' passwords (including `defectdojo_user.password_wo`), 9 to 48, and at most `maximum_password_length`."},
	{name: "maximum_password_length", kind: settingInt, int64Validators: passwordLengthValidators, description: "The maximum length of users' passwords, 9 to 48."},
	{name: "number_character_required", kind: settingBool, description: "Whether users' passwords need a digit."},
	{name: "special_character_required", kind: settingBool, description: "Whether users' passwords need a special character."},
	{name: "lowercase_character_required", kind: settingBool, description: "Whether users' passwords need a lowercase letter."},
	{name: "uppercase_character_required", kind: settingBool, description: "Whether users' passwords need an uppercase letter."},
	{name: "non_common_password_required", kind: settingBool, description: "Whether users' passwords must not be common passwords."},
	{name: "api_expose_error_details", kind: settingBool, description: "Whether API errors include their details."},
	{name: "filter_string_matching", kind: settingBool, description: "Whether the UI's filters match text rather than IDs, which is faster."},
}

// systemSettingsSecret describes a secret system setting, which the resource
// manages through a write-only attribute and its version.
type systemSettingsSecret struct {
	name        string // the attribute name, without _wo
	maxLength   int
	minLength   int
	description string
	clearing    string // how to clear it, if it can be
}

var systemSettingsSecrets = []systemSettingsSecret{
	{name: "slack_token", minLength: 0, maxLength: 100, description: "The token of DefectDojo's Slack app.",
		clearing: "To clear it, set it to `\"\"` with a new version."},
	{name: "msteams_url", minLength: 0, maxLength: 400, description: "The URL of the Microsoft Teams incoming webhook, which carries its token.",
		clearing: "To clear it, set it to `\"\"` with a new version."},
	{name: "jira_webhook_secret", minLength: 1, maxLength: 64,
		description: "The secret that Jira's webhook calls carry in their URL. DefectDojo generates one when it is installed; setting this replaces it, so Jira's webhook URL has to change with it.",
		clearing:    "It can't be cleared from Terraform."},
}

// resourceAttribute returns the resource attribute of the setting:
// Optional+Computed, without a default, so that a setting the configuration
// leaves out keeps DefectDojo's value.
func (s systemSetting) resourceAttribute() schema.Attribute {
	description := s.description + " Read from DefectDojo when not set."
	switch s.kind {
	case settingBool:
		return schema.BoolAttribute{
			MarkdownDescription: description, Optional: true, Computed: true,
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
		}
	case settingInt, settingNullableInt:
		return schema.Int64Attribute{
			MarkdownDescription: description, Optional: true, Computed: true, Validators: append([]validator.Int64{int32Range()}, s.int64Validators...),
			PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
		}
	default:
		validators := append([]validator.String{noSurroundingWhitespace()}, s.stringValidators...)
		if s.maxLength > 0 {
			validators = append(validators, stringvalidator.UTF8LengthAtMost(s.maxLength))
			description = fmt.Sprintf("%s At most %d characters, without leading or trailing whitespace, which DefectDojo removes; `\"\"` clears it.", s.description, s.maxLength) +
				" Read from DefectDojo when not set."
		}
		return schema.StringAttribute{
			MarkdownDescription: description, Optional: true, Computed: true, Validators: validators,
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		}
	}
}

// dataSourceAttribute returns the data source attribute of the setting.
func (s systemSetting) dataSourceAttribute() datasourceschema.Attribute {
	switch s.kind {
	case settingBool:
		return datasourceschema.BoolAttribute{MarkdownDescription: s.description, Computed: true}
	case settingInt, settingNullableInt:
		return datasourceschema.Int64Attribute{MarkdownDescription: s.description, Computed: true}
	default:
		return datasourceschema.StringAttribute{MarkdownDescription: s.description, Computed: true}
	}
}

// zero returns the setting's null value.
func (s systemSetting) zero() attr.Value {
	switch s.kind {
	case settingBool:
		return types.BoolNull()
	case settingInt, settingNullableInt:
		return types.Int64Null()
	default:
		return types.StringNull()
	}
}

// systemSettingsValues returns the settings' values, by JSON name.
func systemSettingsValues(settings *defectdojo.SystemSettings) (map[string]any, error) {
	data, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	var values map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&values); err != nil {
		return nil, err
	}
	return values, nil
}

// value returns the Terraform value of the setting from the values of
// systemSettingsValues.
func (s systemSetting) value(values map[string]any) (attr.Value, error) {
	v, ok := values[s.jsonName()]
	if !ok {
		return nil, fmt.Errorf("the system settings have no %q", s.jsonName())
	}
	if v == nil {
		return s.zero(), nil
	}
	switch s.kind {
	case settingBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("system setting %q is %T, not a boolean", s.jsonName(), v)
		}
		return types.BoolValue(b), nil
	case settingInt, settingNullableInt:
		n, ok := v.(json.Number)
		if !ok {
			return nil, fmt.Errorf("system setting %q is %T, not a number", s.jsonName(), v)
		}
		i, err := n.Int64()
		if err != nil {
			return nil, fmt.Errorf("system setting %q: %w", s.jsonName(), err)
		}
		return types.Int64Value(i), nil
	default:
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("system setting %q is %T, not text", s.jsonName(), v)
		}
		return types.StringValue(str), nil
	}
}

// systemSettingsPatch returns a patch of the settings in values, by JSON name.
// Unmarshalling, rather than setting the fields one by one, keeps the table
// the only list of settings.
func systemSettingsPatch(values map[string]any) (*defectdojo.SystemSettingsPatch, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	patch := new(defectdojo.SystemSettingsPatch)
	if err := json.Unmarshal(data, patch); err != nil {
		return nil, err
	}
	return patch, nil
}
