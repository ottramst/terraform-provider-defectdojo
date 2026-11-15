package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// doximityProvider is the source address of the unmaintained
// doximity/defectdojo provider, without its hostname.
const doximityProvider = "doximity/defectdojo"

// fromDoximity reports whether a state comes from the doximity/defectdojo
// provider: its address with or without the hostname, which the framework
// recommends ignoring.
func fromDoximity(address string) bool {
	return address == doximityProvider || strings.HasSuffix(address, "/"+doximityProvider)
}

// moveFromDoximity returns a state mover that moves a resource of
// sourceType from the doximity/defectdojo provider (v0.0.13 and earlier,
// schema version 0), which stores the DefectDojo ID as a string. It takes only
// the ID: the Read that follows the move in the same plan reads every other
// attribute from DefectDojo, so attributes doximity stored differently (as
// strings, or "" for unset) or not at all don't matter. A state of another
// provider, type or version is left to the other movers, or to Terraform's
// error that the move isn't supported.
func moveFromDoximity(sourceType string) resource.StateMover {
	return resource.StateMover{
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			if !fromDoximity(req.SourceProviderAddress) || req.SourceTypeName != sourceType || req.SourceSchemaVersion != 0 || req.SourceRawState == nil {
				return
			}
			var source struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(req.SourceRawState.JSON, &source); err != nil {
				resp.Diagnostics.AddError("Invalid doximity/defectdojo state",
					fmt.Sprintf("Could not read the %s state of doximity/defectdojo: %s", sourceType, err))
				return
			}
			id, err := strconv.ParseInt(source.ID, 10, 64)
			if err != nil || id <= 0 {
				resp.Diagnostics.AddError("Invalid doximity/defectdojo state",
					fmt.Sprintf("Expected the numeric DefectDojo ID of the %s in the id of doximity/defectdojo's state, got: %q", sourceType, source.ID))
				return
			}
			resp.Diagnostics.Append(resp.TargetState.SetAttribute(ctx, path.Root("id"), id)...)
			resp.Diagnostics.Append(setIDIdentity(ctx, resp.TargetIdentity, types.Int64Value(id))...)
		},
	}
}
