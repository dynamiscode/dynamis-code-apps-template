package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WithClients registers the client and custom-field tools. Bootstrap passes it
// only when the Clients profile is composed, so without it no client tool exists.
// Every tool calls the shared Clients service, which owns authorization.
func WithClients(service *clients.Service) Option {
	return func(h *handler, server *mcp.Server) {
		if service == nil {
			return
		}
		c := clientTools{service: service}
		c.register(h, server)
	}
}

type clientTools struct {
	service *clients.Service
}

// Annotations match the Items tool classes: reads are read-only, writes are
// additive, and permanent removals are destructive.
var (
	clientRead        = &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &falseValue, IdempotentHint: true, OpenWorldHint: &falseValue}
	clientWrite       = &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &falseValue, IdempotentHint: true, OpenWorldHint: &falseValue}
	clientDestructive = &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &trueValue, IdempotentHint: true, OpenWorldHint: &falseValue}
)

// shortIDPattern is the short-ID alphabet. Integer keys and other shapes fail here.
const shortIDPattern = "^[2-9a-hjkmnp-z]{4,}$"

// Client contract types. Bounds mirror the Clients service limits, so the SDK
// refuses out-of-range input before a handler runs.
type (
	displayNameText  string
	emailText        string
	phoneText        string
	notesText        string
	labelText        string
	valueText        string
	sortKey          string
	filterField      string
	filterValue      string
	filterOperator   string
	archivedSelector string
)

var clientContractTypes = withContractTypes(contractTypes, map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[clientShortID]():    {Type: "string", Pattern: shortIDPattern},
	reflect.TypeFor[fieldShortID]():     {Type: "string", Pattern: shortIDPattern},
	reflect.TypeFor[optionShortID]():    {Type: "string", Pattern: shortIDPattern},
	reflect.TypeFor[displayNameText]():  {Type: "string", MinLength: new(1), MaxLength: new(200)},
	reflect.TypeFor[emailText]():        {Type: "string", MaxLength: new(254)},
	reflect.TypeFor[phoneText]():        {Type: "string", MaxLength: new(50)},
	reflect.TypeFor[notesText]():        {Type: "string", MaxLength: new(5000)},
	reflect.TypeFor[labelText]():        {Type: "string", MinLength: new(1), MaxLength: new(100)},
	reflect.TypeFor[valueText]():        {Type: "string", MaxLength: new(500)},
	reflect.TypeFor[sortKey]():          {Type: "string", MaxLength: new(80)},
	reflect.TypeFor[filterField]():      {Type: "string", MaxLength: new(80)},
	reflect.TypeFor[filterValue]():      {Type: "string", MaxLength: new(1000)},
	reflect.TypeFor[filterOperator]():   {Type: "string", Enum: []any{"equals", "contains", "before", "after", "between"}},
	reflect.TypeFor[archivedSelector](): {Type: "string", Enum: []any{"active", "archived", "all"}},
	reflect.TypeFor[clientFieldType]():  {Type: "string", Enum: []any{"text", "number", "date", "yes_no", "single_choice"}},
	reflect.TypeFor[recordVersion]():    {Type: "integer", Minimum: new(1.0)},
})

// withContractTypes returns base with extra added. The base map is not changed.
func withContractTypes(base, extra map[reflect.Type]*jsonschema.Schema) map[reflect.Type]*jsonschema.Schema {
	merged := maps.Clone(base)
	maps.Copy(merged, extra)
	return merged
}

type (
	clientRefInput struct {
		WorkspaceID workspaceShortID `json:"workspaceId"`
		ClientID    clientShortID    `json:"clientId"`
	}
	clientsListInput struct {
		WorkspaceID workspaceShortID `json:"workspaceId"`
		Archived    archivedSelector `json:"archived,omitempty"`
		Sort        sortKey          `json:"sort,omitempty"`
		Filters     []clientFilter   `json:"filters,omitempty"`
		Limit       pageLimit        `json:"limit,omitempty"`
		Page        pageNumber       `json:"page,omitempty"`
	}
	clientFilter struct {
		Field filterField    `json:"field"`
		Op    filterOperator `json:"op"`
		Value filterValue    `json:"value"`
	}
	clientCreateInput struct {
		WorkspaceID workspaceShortID `json:"workspaceId"`
		DisplayName displayNameText  `json:"displayName"`
		Email       emailText        `json:"email,omitempty"`
		Phone       phoneText        `json:"phone,omitempty"`
		Notes       notesText        `json:"notes,omitempty"`
	}
	// Pointers distinguish an absent field from an empty one, so "" clears a value.
	clientUpdateInput struct {
		WorkspaceID workspaceShortID `json:"workspaceId"`
		ClientID    clientShortID    `json:"clientId"`
		Version     recordVersion    `json:"version"`
		DisplayName *displayNameText `json:"displayName,omitempty"`
		Email       *emailText       `json:"email,omitempty"`
		Phone       *phoneText       `json:"phone,omitempty"`
		Notes       *notesText       `json:"notes,omitempty"`
	}
	valueSetInput struct {
		WorkspaceID workspaceShortID `json:"workspaceId"`
		ClientID    clientShortID    `json:"clientId"`
		Version     recordVersion    `json:"version"`
		FieldID     fieldShortID     `json:"fieldId"`
		Value       valueText        `json:"value"`
	}
	valueClearInput struct {
		WorkspaceID workspaceShortID `json:"workspaceId"`
		ClientID    clientShortID    `json:"clientId"`
		Version     recordVersion    `json:"version"`
		FieldID     fieldShortID     `json:"fieldId"`
	}
	fieldsListInput struct {
		WorkspaceID     workspaceShortID `json:"workspaceId"`
		IncludeArchived bool             `json:"includeArchived,omitempty"`
	}
	fieldDefineInput struct {
		WorkspaceID workspaceShortID `json:"workspaceId"`
		Label       labelText        `json:"label"`
		Type        clientFieldType  `json:"type"`
	}
	fieldRefInput struct {
		WorkspaceID workspaceShortID `json:"workspaceId"`
		FieldID     fieldShortID     `json:"fieldId"`
	}
	optionAddInput struct {
		WorkspaceID workspaceShortID `json:"workspaceId"`
		FieldID     fieldShortID     `json:"fieldId"`
		Label       labelText        `json:"label"`
	}
	optionDeleteInput struct {
		WorkspaceID workspaceShortID `json:"workspaceId"`
		FieldID     fieldShortID     `json:"fieldId"`
		OptionID    optionShortID    `json:"optionId"`
	}
)

func (c clientTools) register(h *handler, server *mcp.Server) {
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "clients_list_v1", Description: "List one workspace's clients with filter, sort, archived selector, and bounded page pagination.",
		Annotations: clientRead,
	}, clientContractTypes, func(input, output *jsonschema.Schema) {
		boundArray(input, "filters", 10)
		boundArray(output, "clients", 100)
	}, c.listClients)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "clients_get_v1", Description: "Get one client in an authorized workspace.", Annotations: clientRead,
	}, clientContractTypes, nil, c.getClient)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "clients_create_v1", Description: "Create a client. Members and above may create clients.", Annotations: clientWrite,
	}, clientContractTypes, nil, c.createClient)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "clients_update_v1", Description: "Update a client's common fields when its version matches.", Annotations: clientWrite,
	}, clientContractTypes, requireOneOf("displayName", "email", "phone", "notes"), c.updateClient)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "clients_archive_v1", Description: "Archive a client. Members and above may archive.", Annotations: clientWrite,
	}, clientContractTypes, nil, c.archiveClient)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "clients_unarchive_v1", Description: "Unarchive a client. Members and above may unarchive.", Annotations: clientWrite,
	}, clientContractTypes, nil, c.unarchiveClient)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "clients_delete_v1", Description: "Permanently delete a client and its custom values after client-side human approval. Owners and admins only.",
		Annotations: clientDestructive,
	}, clientContractTypes, nil, c.deleteClient)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "clients_values_list_v1", Description: "List a client's values for the workspace's active custom fields.", Annotations: clientRead,
	}, clientContractTypes, func(_, output *jsonschema.Schema) { boundArray(output, "values", 50) }, c.listValues)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "clients_value_set_v1", Description: "Set one custom field value on a client when the client's version matches.", Annotations: clientWrite,
	}, clientContractTypes, nil, c.setValue)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "clients_value_clear_v1", Description: "Clear one custom field value on a client when the client's version matches.", Annotations: clientWrite,
	}, clientContractTypes, nil, c.clearValue)

	addToolWithTypes(server, h, &mcp.Tool{
		Name: "client_fields_list_v1", Description: "List the workspace's custom field definitions. Archived fields are omitted unless requested.", Annotations: clientRead,
	}, clientContractTypes, func(_, output *jsonschema.Schema) { boundArray(output, "fields", 50) }, c.listFields)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "client_fields_define_v1", Description: "Define a text, number, date, yes/no, or single-choice custom field. Owners and admins only.", Annotations: clientWrite,
	}, clientContractTypes, nil, c.defineField)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "client_fields_archive_v1", Description: "Archive a custom field, keeping its values. Owners and admins only.", Annotations: clientWrite,
	}, clientContractTypes, nil, c.archiveField)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "client_fields_restore_v1", Description: "Restore an archived custom field. Owners and admins only.", Annotations: clientWrite,
	}, clientContractTypes, nil, c.restoreField)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "client_fields_delete_v1", Description: "Permanently delete an archived custom field and clear its values after client-side human approval. Owners and admins only.",
		Annotations: clientDestructive,
	}, clientContractTypes, nil, c.deleteField)

	addToolWithTypes(server, h, &mcp.Tool{
		Name: "client_field_options_list_v1", Description: "List a single-choice field's options.", Annotations: clientRead,
	}, clientContractTypes, func(_, output *jsonschema.Schema) { boundArray(output, "options", 100) }, c.listOptions)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "client_field_options_add_v1", Description: "Add an option to a single-choice field. Owners and admins only.", Annotations: clientWrite,
	}, clientContractTypes, nil, c.addOption)
	addToolWithTypes(server, h, &mcp.Tool{
		Name: "client_field_options_delete_v1", Description: "Delete an option and clear it from every client after client-side human approval. Owners and admins only.",
		Annotations: clientDestructive,
	}, clientContractTypes, nil, c.deleteOption)
}

// boundArray makes one array property a plain array with a maximum length.
func boundArray(schema *jsonschema.Schema, property string, maximum int) {
	list := schema.Properties[property]
	list.Type, list.Types, list.MaxItems = "array", nil, new(maximum)
}

// requireOneOf requires at least one of the named properties, matching the
// service rule that an update changes something.
func requireOneOf(properties ...string) func(input, _ *jsonschema.Schema) {
	return func(input, _ *jsonschema.Schema) {
		for _, property := range properties {
			input.AnyOf = append(input.AnyOf, &jsonschema.Schema{Required: []string{property}})
		}
	}
}

func (c clientTools) workspace(input workspaceShortID) identity.WorkspaceID {
	return identity.ParseWorkspaceIDOrZero(string(input))
}

// onClient records the audit target for a client-scoped tool.
func (r toolRun) onClient(id clients.ClientID) toolRun {
	r.targetType, r.targetKey = "client", int64(id)
	return r
}

func (r toolRun) onField(id clients.FieldID) toolRun {
	r.targetType, r.targetKey = "client_field", int64(id)
	return r
}

func (r toolRun) onOption(id clients.OptionID) toolRun {
	r.targetType, r.targetKey = "client_field_option", int64(id)
	return r
}

func (c clientTools) listClients(ctx context.Context, request *mcp.CallToolRequest, input clientsListInput) (clientPageOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	run := toolRun{workspaceID: workspaceID}
	sortOrder, err := parseClientSort(string(input.Sort))
	if err != nil {
		run.err = err
		return clientPageOutput{}, run
	}
	filters := make([]clients.FilterInput, 0, len(input.Filters))
	for _, filter := range input.Filters {
		filters = append(filters, clients.FilterInput{Field: string(filter.Field), Op: string(filter.Op), Value: string(filter.Value)})
	}
	page, err := c.service.List(ctx, principal(request), workspaceID, clients.ListInput{
		Limit: int(input.Limit), Page: int(input.Page), Sort: sortOrder, Filters: filters, Archived: string(input.Archived),
	})
	run.err = clientRefusal(request, workspaceID, err, errWorkspaceNotFound)
	return toClientPageOutput(page), run
}

// parseClientSort reads "field" or "-field", as the REST list does.
func parseClientSort(raw string) (clients.SortInput, error) {
	if raw == "" {
		return clients.SortInput{}, nil
	}
	field := strings.TrimPrefix(raw, "-")
	if field == "" {
		return clients.SortInput{}, invalidParameters()
	}
	return clients.SortInput{Field: field, Descending: strings.HasPrefix(raw, "-")}, nil
}

func (c clientTools) getClient(ctx context.Context, request *mcp.CallToolRequest, input clientRefInput) (clientOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	clientID := clients.ParseClientIDOrZero(string(input.ClientID))
	client, err := c.service.Get(ctx, principal(request), workspaceID, clientID)
	return toClientOutput(client), toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, clients.ErrNotFound)}.onClient(clientID)
}

func (c clientTools) createClient(ctx context.Context, request *mcp.CallToolRequest, input clientCreateInput) (clientOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	client, err := c.service.Create(ctx, principal(request), workspaceID, clients.Input{
		DisplayName: string(input.DisplayName), Email: string(input.Email), Phone: string(input.Phone), Notes: string(input.Notes),
	}, auditContext(request))
	run := toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, errWorkspaceNotFound)}
	if err == nil {
		run = run.onClient(client.ID)
	}
	return toClientOutput(client), run
}

func (c clientTools) updateClient(ctx context.Context, request *mcp.CallToolRequest, input clientUpdateInput) (clientOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	clientID := clients.ParseClientIDOrZero(string(input.ClientID))
	client, err := c.service.Update(ctx, principal(request), workspaceID, clientID, int64(input.Version), clients.UpdateInput{
		DisplayName: optionalText(input.DisplayName), Email: optionalText(input.Email),
		Phone: optionalText(input.Phone), Notes: optionalText(input.Notes),
	}, auditContext(request))
	return toClientOutput(client), toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, clients.ErrNotFound)}.onClient(clientID)
}

// optionalText turns an optional contract value into the service's pointer form.
func optionalText[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	text := string(*value)
	return &text
}

func (c clientTools) archiveClient(ctx context.Context, request *mcp.CallToolRequest, input clientRefInput) (clientOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	clientID := clients.ParseClientIDOrZero(string(input.ClientID))
	client, err := c.service.Archive(ctx, principal(request), workspaceID, clientID, auditContext(request))
	return toClientOutput(client), toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, clients.ErrNotFound)}.onClient(clientID)
}

func (c clientTools) unarchiveClient(ctx context.Context, request *mcp.CallToolRequest, input clientRefInput) (clientOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	clientID := clients.ParseClientIDOrZero(string(input.ClientID))
	client, err := c.service.Unarchive(ctx, principal(request), workspaceID, clientID, auditContext(request))
	return toClientOutput(client), toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, clients.ErrNotFound)}.onClient(clientID)
}

func (c clientTools) deleteClient(ctx context.Context, request *mcp.CallToolRequest, input clientRefInput) (deletedOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	clientID := clients.ParseClientIDOrZero(string(input.ClientID))
	run := toolRun{workspaceID: workspaceID}.onClient(clientID)
	if pending, err := approvalGate(request, "clients_delete_v1", input,
		fmt.Sprintf("Confirm permanent deletion of client %s and its custom values.", input.ClientID)); pending != nil || err != nil {
		run.pending, run.err = pending, err
		return deletedOutput{}, run
	}
	err := c.service.Delete(ctx, principal(request), workspaceID, clientID, auditContext(request))
	run.err = clientRefusal(request, workspaceID, err, clients.ErrNotFound)
	return deletedOutput{Deleted: err == nil}, run
}

func (c clientTools) listValues(ctx context.Context, request *mcp.CallToolRequest, input clientRefInput) (valuesOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	clientID := clients.ParseClientIDOrZero(string(input.ClientID))
	values, err := c.service.ListValues(ctx, principal(request), workspaceID, clientID)
	output := valuesOutput{Values: make([]valueOutput, 0, len(values))}
	for _, value := range values {
		output.Values = append(output.Values, valueOutput{
			FieldID: fieldShortID(value.FieldID.String()), Label: value.Label, Type: clientFieldType(value.Type), Value: value.Value,
		})
	}
	return output, toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, clients.ErrNotFound)}.onClient(clientID)
}

func (c clientTools) setValue(ctx context.Context, request *mcp.CallToolRequest, input valueSetInput) (clientOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	clientID := clients.ParseClientIDOrZero(string(input.ClientID))
	client, err := c.service.SetValue(ctx, principal(request), workspaceID, clientID, int64(input.Version),
		clients.ParseFieldIDOrZero(string(input.FieldID)), string(input.Value), auditContext(request))
	return toClientOutput(client), toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, clients.ErrFieldNotFound)}.onClient(clientID)
}

func (c clientTools) clearValue(ctx context.Context, request *mcp.CallToolRequest, input valueClearInput) (clientOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	clientID := clients.ParseClientIDOrZero(string(input.ClientID))
	client, err := c.service.ClearValue(ctx, principal(request), workspaceID, clientID, int64(input.Version),
		clients.ParseFieldIDOrZero(string(input.FieldID)), auditContext(request))
	return toClientOutput(client), toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, clients.ErrFieldNotFound)}.onClient(clientID)
}

func (c clientTools) listFields(ctx context.Context, request *mcp.CallToolRequest, input fieldsListInput) (fieldsOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	fields, err := c.service.ListFields(ctx, principal(request), workspaceID, clients.ListFieldsInput{IncludeArchived: input.IncludeArchived})
	output := fieldsOutput{Fields: make([]fieldOutput, 0, len(fields))}
	for _, field := range fields {
		output.Fields = append(output.Fields, toFieldOutput(field))
	}
	return output, toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, errWorkspaceNotFound)}
}

func (c clientTools) defineField(ctx context.Context, request *mcp.CallToolRequest, input fieldDefineInput) (fieldOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	field, err := c.service.DefineField(ctx, principal(request), workspaceID, clients.FieldInput{
		Label: string(input.Label), Type: string(input.Type),
	}, auditContext(request))
	run := toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, errWorkspaceNotFound)}
	if err == nil {
		run = run.onField(field.ID)
	}
	return toFieldOutput(field), run
}

func (c clientTools) archiveField(ctx context.Context, request *mcp.CallToolRequest, input fieldRefInput) (fieldOutput, toolRun) {
	return c.setFieldArchived(ctx, request, input, true)
}

func (c clientTools) restoreField(ctx context.Context, request *mcp.CallToolRequest, input fieldRefInput) (fieldOutput, toolRun) {
	return c.setFieldArchived(ctx, request, input, false)
}

func (c clientTools) setFieldArchived(ctx context.Context, request *mcp.CallToolRequest, input fieldRefInput, archive bool) (fieldOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	fieldID := clients.ParseFieldIDOrZero(string(input.FieldID))
	var (
		field clients.Field
		err   error
	)
	if archive {
		field, err = c.service.ArchiveField(ctx, principal(request), workspaceID, fieldID, auditContext(request))
	} else {
		field, err = c.service.RestoreField(ctx, principal(request), workspaceID, fieldID, auditContext(request))
	}
	return toFieldOutput(field), toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, clients.ErrFieldNotFound)}.onField(fieldID)
}

func (c clientTools) deleteField(ctx context.Context, request *mcp.CallToolRequest, input fieldRefInput) (deletedOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	fieldID := clients.ParseFieldIDOrZero(string(input.FieldID))
	run := toolRun{workspaceID: workspaceID}.onField(fieldID)
	if pending, err := approvalGate(request, "client_fields_delete_v1", input,
		fmt.Sprintf("Confirm permanent deletion of custom field %s and its values on every client.", input.FieldID)); pending != nil || err != nil {
		run.pending, run.err = pending, err
		return deletedOutput{}, run
	}
	err := c.service.DeleteField(ctx, principal(request), workspaceID, fieldID, auditContext(request))
	run.err = clientRefusal(request, workspaceID, err, clients.ErrFieldNotFound)
	return deletedOutput{Deleted: err == nil}, run
}

func (c clientTools) listOptions(ctx context.Context, request *mcp.CallToolRequest, input fieldRefInput) (optionsOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	fieldID := clients.ParseFieldIDOrZero(string(input.FieldID))
	options, err := c.service.ListOptions(ctx, principal(request), workspaceID, fieldID)
	output := optionsOutput{Options: make([]optionOutput, 0, len(options))}
	for _, option := range options {
		output.Options = append(output.Options, toOptionOutput(option))
	}
	return output, toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, clients.ErrFieldNotFound)}
}

func (c clientTools) addOption(ctx context.Context, request *mcp.CallToolRequest, input optionAddInput) (optionOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	fieldID := clients.ParseFieldIDOrZero(string(input.FieldID))
	option, err := c.service.AddOption(ctx, principal(request), workspaceID, fieldID, string(input.Label), auditContext(request))
	run := toolRun{workspaceID: workspaceID, err: clientRefusal(request, workspaceID, err, clients.ErrFieldNotFound)}
	if err == nil {
		run = run.onOption(option.ID)
	}
	return toOptionOutput(option), run
}

func (c clientTools) deleteOption(ctx context.Context, request *mcp.CallToolRequest, input optionDeleteInput) (deletedOutput, toolRun) {
	workspaceID := c.workspace(input.WorkspaceID)
	fieldID := clients.ParseFieldIDOrZero(string(input.FieldID))
	optionID := clients.ParseOptionIDOrZero(string(input.OptionID))
	run := toolRun{workspaceID: workspaceID}.onOption(optionID)
	if pending, err := approvalGate(request, "client_field_options_delete_v1", input,
		fmt.Sprintf("Confirm deletion of choice option %s and its value on every client.", input.OptionID)); pending != nil || err != nil {
		run.pending, run.err = pending, err
		return deletedOutput{}, run
	}
	err := c.service.DeleteOption(ctx, principal(request), workspaceID, fieldID, optionID, auditContext(request))
	run.err = clientRefusal(request, workspaceID, err, clients.ErrFieldNotFound)
	return deletedOutput{Deleted: err == nil}, run
}

// errWorkspaceNotFound stands for a collection call on a workspace the token is
// not bound to, where no record exists to be missing.
var errWorkspaceNotFound = errors.New("workspace not found")

// clientRefusal answers a service error for a call on workspaceID. The service
// refuses a workspace the token is not bound to with the same forbidden error as
// a role refusal. REST answers that case as not found before the service runs
// (workspaceBearer), so here it gets the not-found a missing record gets: missing
// is the sentinel for the record kind. A role refusal in the token's own
// workspace stays access is denied.
func clientRefusal(request *mcp.CallToolRequest, workspaceID identity.WorkspaceID, err, missing error) error {
	if errors.Is(err, identity.ErrForbidden) && principal(request).WorkspaceID != workspaceID {
		return publicClientError(missing)
	}
	return publicClientError(err)
}

// publicClientError maps Clients errors to the safe tool messages. Authorization
// and the shared input errors fall through to the Items mapping, so both surfaces
// answer a refusal the same way.
func publicClientError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errWorkspaceNotFound):
		return &toolError{category: "not_found", message: "workspace not found"}
	case errors.Is(err, clients.ErrNotFound):
		return &toolError{category: "not_found", message: "client not found"}
	case errors.Is(err, clients.ErrFieldNotFound):
		return &toolError{category: "not_found", message: "custom field not found"}
	case errors.Is(err, clients.ErrOptionNotFound):
		return &toolError{category: "not_found", message: "choice option not found"}
	case errors.Is(err, clients.ErrFieldArchived):
		return &toolError{category: "invalid_parameters", message: "custom field is archived"}
	case errors.Is(err, clients.ErrFieldNotArchived):
		return &toolError{category: "conflict", message: "custom field must be archived before it is deleted"}
	case errors.Is(err, clients.ErrLimit):
		return &toolError{category: "resource_limit", message: "workspace custom field limit reached"}
	case errors.Is(err, clients.ErrOptionLimit):
		return &toolError{category: "resource_limit", message: "choice option limit reached"}
	case errors.Is(err, clients.ErrPreconditionFailed):
		return &toolError{category: "precondition_failed", message: "client version is stale"}
	case errors.Is(err, clients.ErrInvalidInput):
		return invalidParameters()
	default:
		return publicError(err)
	}
}

func toClientOutput(client clients.Client) clientOutput {
	return clientOutput{
		ID: clientShortID(client.ID.String()), WorkspaceID: workspaceShortID(client.WorkspaceID.String()),
		CreatedByUserID: optionalUserShortID(client.CreatedByUserID), DisplayName: client.DisplayName,
		Email: client.Email, Phone: client.Phone, Notes: client.Notes, ArchivedAt: client.ArchivedAt,
		Version: recordVersion(client.Version), CreatedAt: client.CreatedAt, UpdatedAt: client.UpdatedAt,
	}
}

func toClientPageOutput(page clients.Page) clientPageOutput {
	output := clientPageOutput{Clients: make([]clientOutput, 0, len(page.Clients)), Total: page.Total, Page: page.Page}
	for _, client := range page.Clients {
		output.Clients = append(output.Clients, toClientOutput(client))
	}
	return output
}

func toFieldOutput(field clients.Field) fieldOutput {
	return fieldOutput{
		ID: fieldShortID(field.ID.String()), WorkspaceID: workspaceShortID(field.WorkspaceID.String()),
		Label: field.Label, Type: clientFieldType(field.Type), ArchivedAt: field.ArchivedAt,
		CreatedAt: field.CreatedAt, UpdatedAt: field.UpdatedAt,
	}
}

func toOptionOutput(option clients.Option) optionOutput {
	return optionOutput{
		ID: optionShortID(option.ID.String()), FieldID: fieldShortID(option.FieldID.String()),
		Label: option.Label, CreatedAt: option.CreatedAt,
	}
}

// Output and contract types. Identifiers are short IDs, never integer keys.
type (
	clientShortID   string
	fieldShortID    string
	optionShortID   string
	clientFieldType string
	recordVersion   int64
)

type (
	clientOutput struct {
		ID              clientShortID    `json:"id"`
		WorkspaceID     workspaceShortID `json:"workspaceId"`
		CreatedByUserID *userShortID     `json:"createdByUserId"`
		DisplayName     string           `json:"displayName"`
		Email           string           `json:"email"`
		Phone           string           `json:"phone"`
		Notes           string           `json:"notes"`
		ArchivedAt      *time.Time       `json:"archivedAt"`
		Version         recordVersion    `json:"version"`
		CreatedAt       time.Time        `json:"createdAt"`
		UpdatedAt       time.Time        `json:"updatedAt"`
	}
	clientPageOutput struct {
		Clients []clientOutput `json:"clients"`
		Total   int            `json:"total"`
		Page    int            `json:"page"`
	}
	valueOutput struct {
		FieldID fieldShortID    `json:"fieldId"`
		Label   string          `json:"label"`
		Type    clientFieldType `json:"type"`
		Value   *string         `json:"value"`
	}
	valuesOutput struct {
		Values []valueOutput `json:"values"`
	}
	fieldOutput struct {
		ID          fieldShortID     `json:"id"`
		WorkspaceID workspaceShortID `json:"workspaceId"`
		Label       string           `json:"label"`
		Type        clientFieldType  `json:"type"`
		ArchivedAt  *time.Time       `json:"archivedAt"`
		CreatedAt   time.Time        `json:"createdAt"`
		UpdatedAt   time.Time        `json:"updatedAt"`
	}
	fieldsOutput struct {
		Fields []fieldOutput `json:"fields"`
	}
	optionOutput struct {
		ID        optionShortID `json:"id"`
		FieldID   fieldShortID  `json:"fieldId"`
		Label     string        `json:"label"`
		CreatedAt time.Time     `json:"createdAt"`
	}
	optionsOutput struct {
		Options []optionOutput `json:"options"`
	}
	deletedOutput struct {
		Deleted bool `json:"deleted"`
	}
)
