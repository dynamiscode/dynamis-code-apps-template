package mcpserver

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/transactions"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WithTransactions registers transactions tools only when the Transactions profile is composed.
func WithTransactions(service *transactions.Service) Option {
	return func(h *handler, server *mcp.Server) {
		if service == nil {
			return
		}
		tools := transactionsTools{service: service}
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_list_v1", Description: "List a page of workspace transactions entries.", Annotations: clientRead}, transactionsContractTypes, func(_, output *jsonschema.Schema) { boundArray(output, "entries", 100) }, tools.list)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_totals_v1", Description: "Page through workspace transactions currency totals and method and category breakdowns. Owners and admins only.", Annotations: clientRead}, transactionsContractTypes, func(_, output *jsonschema.Schema) { boundArray(output, "items", 100) }, tools.totals)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_methods_v1", Description: "List enabled workspace transactions payment methods.", Annotations: clientRead}, transactionsContractTypes, func(_, output *jsonschema.Schema) { boundArray(output, "methods", 100) }, tools.methods)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_method_list_v1", Description: "Page through payment method IDs, names, and enabled states; disabled methods are visible to owners and admins.", Annotations: clientRead}, transactionsContractTypes, func(_, output *jsonschema.Schema) { boundArray(output, "methods", 100) }, tools.listMethods)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_method_create_v1", Description: "Create an enabled payment method. Owners and admins only.", Annotations: &mcp.ToolAnnotations{DestructiveHint: &falseValue, OpenWorldHint: &falseValue}}, transactionsContractTypes, nil, tools.createMethod)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_method_update_v1", Description: "Rename or enable or disable a payment method. Owners and admins only.", Annotations: clientWrite}, transactionsContractTypes, nil, tools.updateMethod)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_create_v1", Description: "Create an immutable workspace transactions entry.", Annotations: &mcp.ToolAnnotations{DestructiveHint: &falseValue, OpenWorldHint: &falseValue}}, transactionsContractTypes, nil, tools.create)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_categories_v1", Description: "List workspace expense categories.", Annotations: clientRead}, transactionsContractTypes, func(_, output *jsonschema.Schema) { boundArray(output, "categories", 100) }, tools.categories)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_category_create_v1", Description: "Create an expense category.", Annotations: &mcp.ToolAnnotations{DestructiveHint: &falseValue, OpenWorldHint: &falseValue}}, transactionsContractTypes, nil, tools.categoryCreate)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_category_rename_v1", Description: "Rename an expense category.", Annotations: &mcp.ToolAnnotations{DestructiveHint: &falseValue, OpenWorldHint: &falseValue}}, transactionsContractTypes, nil, tools.categoryRename)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_category_archive_v1", Description: "Archive an expense category.", Annotations: &mcp.ToolAnnotations{OpenWorldHint: &falseValue}}, transactionsContractTypes, nil, tools.categoryArchive)
		addToolWithTypes(server, h, &mcp.Tool{Name: "transactions_void_v1", Description: "Void a workspace transactions entry after client-side human approval. Owners and admins only.", Annotations: &mcp.ToolAnnotations{DestructiveHint: &trueValue, OpenWorldHint: &falseValue}}, transactionsContractTypes, nil, tools.void)
	}
}

type transactionsTools struct{ service *transactions.Service }
type transactionsDirection string
type transactionsCurrency string
type transactionsMethod string
type transactionsDate string
type transactionsDescription string
type transactionsOutputDescription string
type transactionsNote string
type transactionsVoidReason string
type transactionsAmount int64
type transactionsEntryID string
type transactionsCategoryID string
type transactionsCategoryName string
type transactionsPeriod string
type transactionsPeriodDate string
type transactionsTotalsCursor string
type transactionsTotalsKind string
type transactionsTotalsValue string
type transactionsTotalsID string
type transactionsTotalsName string

var transactionsContractTypes = withContractTypes(clientContractTypes, map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[workspaceShortID]():              {Type: "string", Pattern: shortIDPattern, MaxLength: new(32)},
	reflect.TypeFor[clientShortID]():                 {Type: "string", Pattern: shortIDPattern, MaxLength: new(32)},
	reflect.TypeFor[transactionsEntryID]():           {Type: "string", Pattern: shortIDPattern, MaxLength: new(32)},
	reflect.TypeFor[transactionsCategoryID]():        {Type: "string", Pattern: shortIDPattern, MaxLength: new(32)},
	reflect.TypeFor[transactionsCategoryName]():      {Type: "string", MinLength: new(1), MaxLength: new(64)},
	reflect.TypeFor[transactionsDirection]():         {Type: "string", Enum: []any{"in", "out"}},
	reflect.TypeFor[transactionsCurrency]():          {Type: "string", Pattern: "^[A-Za-z]{3}$"},
	reflect.TypeFor[transactionsMethod]():            {Type: "string", MinLength: new(1), MaxLength: new(64)},
	reflect.TypeFor[transactionsDate]():              {Type: "string", Pattern: "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
	reflect.TypeFor[transactionsPeriod]():            {Type: "string", Enum: []any{"", "this_month", "last_month", "custom"}},
	reflect.TypeFor[transactionsPeriodDate]():        {Type: "string", Pattern: "^([0-9]{4}-[0-9]{2}-[0-9]{2})?$"},
	reflect.TypeFor[transactionsTotalsCursor]():      {Type: "string", Pattern: "^([A-Z]{3}:[012]:[a-z0-9_-]{0,64})?$", MaxLength: new(71)},
	reflect.TypeFor[transactionsTotalsKind]():        {Type: "string", Enum: []any{"currency", "method", "category"}},
	reflect.TypeFor[transactionsTotalsValue]():       {Type: "string", Pattern: "^-?[0-9]+$", MaxLength: new(40)},
	reflect.TypeFor[transactionsTotalsID]():          {Type: "string", MaxLength: new(64)},
	reflect.TypeFor[transactionsTotalsName]():        {Type: "string", MaxLength: new(64)},
	reflect.TypeFor[transactionsDescription]():       {Type: "string", MinLength: new(1), MaxLength: new(200)},
	reflect.TypeFor[transactionsOutputDescription](): {Type: "string", MaxLength: new(200)},
	reflect.TypeFor[transactionsNote]():              {Type: "string", MaxLength: new(1000)},
	reflect.TypeFor[transactionsVoidReason]():        {Type: "string", MaxLength: new(1000)},
	reflect.TypeFor[transactionsAmount]():            {Type: "integer", Minimum: new(1.0)},
})

type transactionsWorkspaceInput struct {
	WorkspaceID workspaceShortID `json:"workspaceId"`
}
type transactionsMethodsInput struct {
	WorkspaceID workspaceShortID   `json:"workspaceId"`
	Cursor      transactionsMethod `json:"cursor,omitempty"`
}
type transactionsListInput struct {
	WorkspaceID   workspaceShortID       `json:"workspaceId"`
	Page          pageNumber             `json:"page,omitempty"`
	ClientID      clientShortID          `json:"clientId,omitempty"`
	CategoryID    transactionsCategoryID `json:"categoryId,omitempty"`
	PaymentMethod transactionsMethod     `json:"paymentMethod,omitempty"`
	Direction     transactionsDirection  `json:"direction,omitempty"`
	From          transactionsDate       `json:"from,omitempty"`
	To            transactionsDate       `json:"to,omitempty"`
}
type transactionsCategoriesInput struct {
	WorkspaceID workspaceShortID `json:"workspaceId"`
	Page        pageNumber       `json:"page,omitempty"`
}
type transactionsTotalsInput struct {
	WorkspaceID workspaceShortID         `json:"workspaceId"`
	Period      transactionsPeriod       `json:"period,omitempty"`
	Start       transactionsPeriodDate   `json:"start,omitempty"`
	End         transactionsPeriodDate   `json:"end,omitempty"`
	Limit       pageLimit                `json:"limit,omitempty"`
	Cursor      transactionsTotalsCursor `json:"cursor,omitempty"`
}

type transactionsTotalsItem struct {
	Kind          transactionsTotalsKind  `json:"kind"`
	Currency      transactionsCurrency    `json:"currency"`
	ID            transactionsTotalsID    `json:"id,omitempty"`
	Name          transactionsTotalsName  `json:"name,omitempty"`
	Archived      bool                    `json:"archived,omitempty"`
	Uncategorized bool                    `json:"uncategorized,omitempty"`
	MoneyIn       transactionsTotalsValue `json:"moneyIn,omitempty"`
	MoneyOut      transactionsTotalsValue `json:"moneyOut"`
	Net           transactionsTotalsValue `json:"net,omitempty"`
}

type transactionsTotalsOutput struct {
	Period     transactionsPeriod       `json:"period"`
	Start      transactionsDate         `json:"start"`
	End        transactionsDate         `json:"end"`
	Items      []transactionsTotalsItem `json:"items"`
	NextCursor transactionsTotalsCursor `json:"nextCursor,omitempty"`
}
type transactionsMethodCreateInput struct {
	WorkspaceID workspaceShortID   `json:"workspaceId"`
	Name        transactionsMethod `json:"name"`
}
type transactionsMethodUpdateInput struct {
	WorkspaceID workspaceShortID   `json:"workspaceId"`
	ID          transactionsMethod `json:"id"`
	Name        transactionsMethod `json:"name"`
	Enabled     bool               `json:"enabled"`
}
type transactionsMethodOutput struct {
	ID      transactionsMethod `json:"id"`
	Name    transactionsMethod `json:"name"`
	Enabled bool               `json:"enabled"`
}
type transactionsMethodListOutput struct {
	Methods    []transactionsMethodOutput `json:"methods"`
	NextCursor transactionsMethod         `json:"nextCursor,omitempty"`
}
type transactionsCreateInput struct {
	WorkspaceID     workspaceShortID        `json:"workspaceId"`
	Description     transactionsDescription `json:"description"`
	Direction       transactionsDirection   `json:"direction"`
	AmountMinor     transactionsAmount      `json:"amountMinor"`
	Currency        transactionsCurrency    `json:"currency"`
	PaymentMethod   transactionsMethod      `json:"paymentMethod"`
	OccurredOn      transactionsDate        `json:"occurredOn,omitempty"`
	ClientID        *clientShortID          `json:"clientId,omitempty"`
	CategoryID      *transactionsCategoryID `json:"categoryId,omitempty"`
	Note            transactionsNote        `json:"note,omitempty"`
	ReplacesEntryID *transactionsEntryID    `json:"replacesEntryId,omitempty"`
	RefundOfEntryID *transactionsEntryID    `json:"refundOfEntryId,omitempty"`
}
type transactionsVoidInput struct {
	WorkspaceID workspaceShortID       `json:"workspaceId"`
	EntryID     transactionsEntryID    `json:"entryId"`
	Reason      transactionsVoidReason `json:"reason,omitempty"`
}
type transactionsEntryOutput struct {
	ID              transactionsEntryID           `json:"id"`
	Description     transactionsOutputDescription `json:"description"`
	Direction       transactionsDirection         `json:"direction"`
	AmountMinor     transactionsAmount            `json:"amountMinor"`
	Currency        transactionsCurrency          `json:"currency"`
	PaymentMethod   transactionsMethod            `json:"paymentMethod"`
	OccurredOn      transactionsDate              `json:"occurredOn"`
	ClientID        *clientShortID                `json:"clientId"`
	CategoryID      *transactionsCategoryID       `json:"categoryId"`
	CategoryName    string                        `json:"categoryName"`
	Note            transactionsNote              `json:"note"`
	CreatedBy       userShortID                   `json:"createdBy"`
	CreatedAt       time.Time                     `json:"createdAt"`
	VoidedBy        *userShortID                  `json:"voidedBy"`
	VoidedAt        string                        `json:"voidedAt"`
	VoidReason      string                        `json:"voidReason"`
	ReplacesEntryID *transactionsEntryID          `json:"replacesEntryId"`
	RefundOfEntryID *transactionsEntryID          `json:"refundOfEntryId"`
}
type transactionsListOutput struct {
	Entries []transactionsEntryOutput `json:"entries"`
	Page    int                       `json:"page"`
}
type transactionsMethodsOutput struct {
	Methods    []transactionsMethod `json:"methods"`
	NextCursor transactionsMethod   `json:"nextCursor,omitempty"`
}

func transactionsOutput(entry transactions.Entry) transactionsEntryOutput {
	var clientID *clientShortID
	if entry.ClientID != nil {
		value := clientShortID(entry.ClientID.String())
		clientID = &value
	}
	var categoryID *transactionsCategoryID
	if entry.CategoryID != nil {
		value := transactionsCategoryID(entry.CategoryID.String())
		categoryID = &value
	}
	output := transactionsEntryOutput{ID: transactionsEntryID(entry.ID.String()), Description: transactionsOutputDescription(entry.Description), Direction: transactionsDirection(entry.Direction), AmountMinor: transactionsAmount(entry.AmountMinor), Currency: transactionsCurrency(entry.Currency), PaymentMethod: transactionsMethod(entry.PaymentMethod), OccurredOn: transactionsDate(entry.OccurredOn), ClientID: clientID, CategoryID: categoryID, CategoryName: entry.CategoryName, Note: transactionsNote(entry.Note), CreatedBy: userShortID(entry.CreatedBy.String()), CreatedAt: entry.CreatedAt, VoidedAt: entry.VoidedAt, VoidReason: entry.VoidReason}
	if entry.VoidedBy != 0 {
		value := userShortID(entry.VoidedBy.String())
		output.VoidedBy = &value
	}
	if entry.ReplacesEntryID != 0 {
		value := transactionsEntryID(entry.ReplacesEntryID.String())
		output.ReplacesEntryID = &value
	}
	if entry.RefundOfEntryID != 0 {
		value := transactionsEntryID(entry.RefundOfEntryID.String())
		output.RefundOfEntryID = &value
	}
	return output
}

func transactionsRefusal(request *mcp.CallToolRequest, workspaceID identity.WorkspaceID, err error) error {
	if errors.Is(err, transactions.ErrInvalidInput) {
		return invalidParameters()
	}
	if errors.Is(err, transactions.ErrLastMethod) {
		return &toolError{category: "conflict", message: "cannot disable the last enabled payment method"}
	}
	return clientRefusal(request, workspaceID, err, errWorkspaceNotFound)
}

func (tools transactionsTools) list(ctx context.Context, request *mcp.CallToolRequest, input transactionsListInput) (transactionsListOutput, toolRun) {
	workspaceID := identity.ParseWorkspaceIDOrZero(string(input.WorkspaceID))
	page := int(input.Page)
	if page == 0 {
		page = 1
	}
	filter := transactions.ListFilter{Page: page, PaymentMethod: string(input.PaymentMethod), Direction: string(input.Direction), From: string(input.From), To: string(input.To)}
	if input.ClientID != "" {
		clientID, err := clients.ParseClientID(string(input.ClientID))
		if err != nil {
			return transactionsListOutput{}, toolRun{workspaceID: workspaceID, err: transactionsRefusal(request, workspaceID, transactions.ErrInvalidInput)}
		}
		filter.ClientID = clientID
	}
	if input.CategoryID != "" {
		categoryID, err := transactions.ParseCategoryID(string(input.CategoryID))
		if err != nil {
			return transactionsListOutput{}, toolRun{workspaceID: workspaceID, err: transactionsRefusal(request, workspaceID, transactions.ErrInvalidInput)}
		}
		filter.CategoryID = categoryID
	}
	entries, err := tools.service.List(ctx, principal(request), workspaceID, filter)
	output := transactionsListOutput{Entries: make([]transactionsEntryOutput, 0, len(entries)), Page: page}
	for _, entry := range entries {
		output.Entries = append(output.Entries, transactionsOutput(entry))
	}
	return output, toolRun{workspaceID: workspaceID, err: transactionsRefusal(request, workspaceID, err)}
}

func (tools transactionsTools) totals(ctx context.Context, request *mcp.CallToolRequest, input transactionsTotalsInput) (transactionsTotalsOutput, toolRun) {
	workspaceID := identity.ParseWorkspaceIDOrZero(string(input.WorkspaceID))
	totals, err := tools.service.Totals(ctx, principal(request), workspaceID, string(input.Period), string(input.Start), string(input.End))
	run := toolRun{workspaceID: workspaceID, err: transactionsRefusal(request, workspaceID, err)}
	if err != nil {
		return transactionsTotalsOutput{}, run
	}
	limit := int(input.Limit)
	if limit == 0 {
		limit = 50
	}
	output := transactionsTotalsOutput{Period: transactionsPeriod(totals.Period), Start: transactionsDate(totals.Start), End: transactionsDate(totals.End), Items: make([]transactionsTotalsItem, 0, limit)}
	sort.Slice(totals.Currencies, func(i, j int) bool { return totals.Currencies[i].Currency < totals.Currencies[j].Currency })
	lastKey := ""
	add := func(key string, item transactionsTotalsItem) bool {
		if key <= string(input.Cursor) {
			return true
		}
		if len(output.Items) == limit {
			output.NextCursor = transactionsTotalsCursor(lastKey)
			return false
		}
		output.Items = append(output.Items, item)
		lastKey = key
		return true
	}
	for _, currency := range totals.Currencies {
		prefix := currency.Currency + ":"
		if !add(prefix+"0:", transactionsTotalsItem{Kind: "currency", Currency: transactionsCurrency(currency.Currency), MoneyIn: transactionsTotalsValue(currency.MoneyIn), MoneyOut: transactionsTotalsValue(currency.MoneyOut), Net: transactionsTotalsValue(currency.Net)}) {
			break
		}
		sort.Slice(currency.Methods, func(i, j int) bool { return currency.Methods[i].ID < currency.Methods[j].ID })
		for _, method := range currency.Methods {
			if !add(prefix+"1:"+method.ID, transactionsTotalsItem{Kind: "method", Currency: transactionsCurrency(currency.Currency), ID: transactionsTotalsID(method.ID), Name: transactionsTotalsName(method.Name), MoneyIn: transactionsTotalsValue(method.MoneyIn), MoneyOut: transactionsTotalsValue(method.MoneyOut), Net: transactionsTotalsValue(method.Net)}) {
				return output, run
			}
		}
		categoryKey := func(category transactions.CategoryTotal) string {
			if category.ID == nil {
				return ""
			}
			return category.ID.String()
		}
		sort.Slice(currency.Categories, func(i, j int) bool { return categoryKey(currency.Categories[i]) < categoryKey(currency.Categories[j]) })
		for _, category := range currency.Categories {
			key := categoryKey(category)
			if !add(prefix+"2:"+key, transactionsTotalsItem{Kind: "category", Currency: transactionsCurrency(currency.Currency), ID: transactionsTotalsID(key), Name: transactionsTotalsName(category.Name), Archived: category.Archived, Uncategorized: category.Uncategorized, MoneyOut: transactionsTotalsValue(category.MoneyOut)}) {
				return output, run
			}
		}
	}
	return output, run
}

func (tools transactionsTools) methods(ctx context.Context, request *mcp.CallToolRequest, input transactionsMethodsInput) (transactionsMethodsOutput, toolRun) {
	workspaceID := identity.ParseWorkspaceIDOrZero(string(input.WorkspaceID))
	methods, err := tools.service.ListMethodPage(ctx, principal(request), workspaceID, string(input.Cursor), true)
	run := toolRun{workspaceID: workspaceID, err: transactionsRefusal(request, workspaceID, err)}
	if err != nil {
		return transactionsMethodsOutput{}, run
	}
	output := transactionsMethodsOutput{Methods: make([]transactionsMethod, 0, 100)}
	for _, method := range methods {
		if len(output.Methods) == 100 {
			output.NextCursor = output.Methods[len(output.Methods)-1]
			break
		}
		output.Methods = append(output.Methods, transactionsMethod(method.ID))
	}
	return output, run
}

func (tools transactionsTools) listMethods(ctx context.Context, request *mcp.CallToolRequest, input transactionsMethodsInput) (transactionsMethodListOutput, toolRun) {
	workspaceID := identity.ParseWorkspaceIDOrZero(string(input.WorkspaceID))
	methods, err := tools.service.ListMethodPage(ctx, principal(request), workspaceID, string(input.Cursor), false)
	run := toolRun{workspaceID: workspaceID, err: transactionsRefusal(request, workspaceID, err)}
	if err != nil {
		return transactionsMethodListOutput{}, run
	}
	output := transactionsMethodListOutput{Methods: make([]transactionsMethodOutput, 0, 100)}
	for _, method := range methods {
		if len(output.Methods) == 100 {
			output.NextCursor = output.Methods[len(output.Methods)-1].ID
			break
		}
		output.Methods = append(output.Methods, transactionsMethodOutput{ID: transactionsMethod(method.ID), Name: transactionsMethod(method.Name), Enabled: method.Enabled})
	}
	return output, run
}

func (tools transactionsTools) createMethod(ctx context.Context, request *mcp.CallToolRequest, input transactionsMethodCreateInput) (transactionsMethodOutput, toolRun) {
	workspaceID := identity.ParseWorkspaceIDOrZero(string(input.WorkspaceID))
	method, err := tools.service.SaveMethod(ctx, principal(request), workspaceID, "", string(input.Name), true)
	return transactionsMethodOutput{ID: transactionsMethod(method.ID), Name: transactionsMethod(method.Name), Enabled: method.Enabled}, toolRun{workspaceID: workspaceID, err: transactionsRefusal(request, workspaceID, err)}
}

func (tools transactionsTools) updateMethod(ctx context.Context, request *mcp.CallToolRequest, input transactionsMethodUpdateInput) (transactionsMethodOutput, toolRun) {
	workspaceID := identity.ParseWorkspaceIDOrZero(string(input.WorkspaceID))
	method, err := tools.service.SaveMethod(ctx, principal(request), workspaceID, string(input.ID), string(input.Name), input.Enabled)
	return transactionsMethodOutput{ID: transactionsMethod(method.ID), Name: transactionsMethod(method.Name), Enabled: method.Enabled}, toolRun{workspaceID: workspaceID, err: transactionsRefusal(request, workspaceID, err)}
}

func (tools transactionsTools) create(ctx context.Context, request *mcp.CallToolRequest, input transactionsCreateInput) (transactionsEntryOutput, toolRun) {
	workspaceID := identity.ParseWorkspaceIDOrZero(string(input.WorkspaceID))
	var clientID *clients.ClientID
	if input.ClientID != nil {
		value := clients.ParseClientIDOrZero(string(*input.ClientID))
		clientID = &value
	}
	var categoryID *transactions.CategoryID
	if input.CategoryID != nil {
		value, _ := transactions.ParseCategoryID(string(*input.CategoryID))
		categoryID = &value
	}
	var replacesID, refundID transactions.EntryID
	for _, link := range []struct {
		input  *transactionsEntryID
		output *transactions.EntryID
	}{{input.ReplacesEntryID, &replacesID}, {input.RefundOfEntryID, &refundID}} {
		if link.input != nil {
			key, err := id.DecodeKey(id.TypeTransactionsEntry, string(*link.input))
			if err != nil {
				return transactionsEntryOutput{}, toolRun{workspaceID: workspaceID, err: invalidParameters()}
			}
			*link.output = transactions.EntryID(key)
		}
	}
	entry, err := tools.service.Create(ctx, principal(request), workspaceID, transactions.Input{Description: string(input.Description), Direction: string(input.Direction), AmountMinor: int64(input.AmountMinor), Currency: string(input.Currency), PaymentMethod: string(input.PaymentMethod), OccurredOn: string(input.OccurredOn), ClientID: clientID, CategoryID: categoryID, Note: string(input.Note), ReplacesEntryID: replacesID, RefundOfEntryID: refundID})
	run := toolRun{workspaceID: workspaceID, err: transactionsRefusal(request, workspaceID, err)}
	if err == nil {
		run.targetType, run.targetKey = "transactions_entry", int64(entry.ID)
	}
	return transactionsOutput(entry), run
}

type transactionsCategoryOutput struct {
	ID       transactionsCategoryID   `json:"id"`
	Name     transactionsCategoryName `json:"name"`
	Archived bool                     `json:"archived"`
}
type transactionsCategoriesOutput struct {
	Categories []transactionsCategoryOutput `json:"categories"`
	Page       int                          `json:"page"`
}
type transactionsCategoryCreateInput struct {
	WorkspaceID workspaceShortID         `json:"workspaceId"`
	Name        transactionsCategoryName `json:"name"`
}
type transactionsCategoryChangeInput struct {
	WorkspaceID workspaceShortID         `json:"workspaceId"`
	CategoryID  transactionsCategoryID   `json:"categoryId"`
	Name        transactionsCategoryName `json:"name"`
}
type transactionsCategoryArchiveInput struct {
	WorkspaceID workspaceShortID       `json:"workspaceId"`
	CategoryID  transactionsCategoryID `json:"categoryId"`
}

func categoryOutput(c transactions.Category) transactionsCategoryOutput {
	return transactionsCategoryOutput{ID: transactionsCategoryID(c.ID.String()), Name: transactionsCategoryName(c.Name), Archived: c.Archived}
}
func (tools transactionsTools) categories(ctx context.Context, request *mcp.CallToolRequest, input transactionsCategoriesInput) (transactionsCategoriesOutput, toolRun) {
	workspace := identity.ParseWorkspaceIDOrZero(string(input.WorkspaceID))
	page := int(input.Page)
	if page == 0 {
		page = 1
	}
	if page < 1 || page > 1000000 {
		return transactionsCategoriesOutput{}, toolRun{workspaceID: workspace, err: invalidParameters()}
	}
	categories, err := tools.service.Categories(ctx, principal(request), workspace)
	output := transactionsCategoriesOutput{Categories: []transactionsCategoryOutput{}, Page: page}
	start := (page - 1) * 100
	for i := start; i < len(categories) && i < start+100; i++ {
		output.Categories = append(output.Categories, categoryOutput(categories[i]))
	}
	return output, toolRun{workspaceID: workspace, err: transactionsRefusal(request, workspace, err)}
}
func (tools transactionsTools) categoryChange(ctx context.Context, request *mcp.CallToolRequest, workspaceID workspaceShortID, categoryID transactionsCategoryID, name transactionsCategoryName, archive bool) (transactionsCategoryOutput, toolRun) {
	workspace := identity.ParseWorkspaceIDOrZero(string(workspaceID))
	var key transactions.CategoryID
	if categoryID != "" {
		var err error
		key, err = transactions.ParseCategoryID(string(categoryID))
		if err != nil {
			return transactionsCategoryOutput{}, toolRun{workspaceID: workspace, err: invalidParameters()}
		}
	}
	c, err := tools.service.ChangeCategory(ctx, principal(request), workspace, key, string(name), archive)
	run := toolRun{workspaceID: workspace, err: transactionsRefusal(request, workspace, err)}
	if err == nil {
		run.targetType, run.targetKey = "transactions_category", int64(c.ID)
	}
	return categoryOutput(c), run
}
func (tools transactionsTools) categoryCreate(ctx context.Context, request *mcp.CallToolRequest, input transactionsCategoryCreateInput) (transactionsCategoryOutput, toolRun) {
	return tools.categoryChange(ctx, request, input.WorkspaceID, "", input.Name, false)
}
func (tools transactionsTools) categoryRename(ctx context.Context, request *mcp.CallToolRequest, input transactionsCategoryChangeInput) (transactionsCategoryOutput, toolRun) {
	return tools.categoryChange(ctx, request, input.WorkspaceID, input.CategoryID, input.Name, false)
}
func (tools transactionsTools) categoryArchive(ctx context.Context, request *mcp.CallToolRequest, input transactionsCategoryArchiveInput) (transactionsCategoryOutput, toolRun) {
	return tools.categoryChange(ctx, request, input.WorkspaceID, input.CategoryID, "", true)
}

func (tools transactionsTools) void(ctx context.Context, request *mcp.CallToolRequest, input transactionsVoidInput) (transactionsEntryOutput, toolRun) {
	workspaceID := identity.ParseWorkspaceIDOrZero(string(input.WorkspaceID))
	key, err := id.DecodeKey(id.TypeTransactionsEntry, string(input.EntryID))
	run := toolRun{workspaceID: workspaceID, targetType: "transactions_entry", targetKey: key}
	if err != nil {
		run.err = invalidParameters()
		return transactionsEntryOutput{}, run
	}
	if pending, err := approvalGate(request, "transactions_void_v1", input, "Confirm voiding transactions entry "+string(input.EntryID)+"."); pending != nil || err != nil {
		run.pending, run.err = pending, err
		return transactionsEntryOutput{}, run
	}
	entry, err := tools.service.Void(ctx, principal(request), workspaceID, transactions.EntryID(key), string(input.Reason))
	run.err = transactionsRefusal(request, workspaceID, err)
	return transactionsOutput(entry), run
}
