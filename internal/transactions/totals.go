package transactions

import (
	"context"
	"database/sql"
	"math/big"
	"sort"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

type Totals struct {
	Period     string           `json:"period"`
	Start      string           `json:"start"`
	End        string           `json:"end"`
	Currencies []CurrencyTotals `json:"currencies"`
}

type CurrencyTotals struct {
	Currency   string          `json:"currency"`
	MoneyIn    string          `json:"moneyIn"`
	MoneyOut   string          `json:"moneyOut"`
	Net        string          `json:"net"`
	Methods    []MethodTotal   `json:"methods"`
	Categories []CategoryTotal `json:"categories"`
}

type MethodTotal struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MoneyIn  string `json:"moneyIn"`
	MoneyOut string `json:"moneyOut"`
	Net      string `json:"net"`
}

type CategoryTotal struct {
	ID            *CategoryID `json:"id"`
	Name          string      `json:"name"`
	Archived      bool        `json:"archived"`
	MoneyOut      string      `json:"moneyOut"`
	Uncategorized bool        `json:"uncategorized"`
}

// Totals reports separate currency balances over inclusive workspace-local dates.
func (s *Service) Totals(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, period, start, end string) (Totals, error) {
	principal, err := s.auth.AuthorizePrincipal(ctx, actor, workspace, identity.ResourcesRead)
	if err != nil {
		return Totals{}, err
	}
	if principal.Role != identity.Owner && principal.Role != identity.Admin {
		return Totals{}, identity.ErrForbidden
	}
	if period == "" {
		period = "this_month"
	}
	switch period {
	case "this_month", "last_month":
		date, err := today(ctx, s.db, workspace, time.Now())
		if err != nil {
			return Totals{}, err
		}
		month, err := time.Parse("2006-01-02", date)
		if err != nil {
			return Totals{}, err
		}
		month = time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
		if period == "last_month" {
			month = month.AddDate(0, -1, 0)
		}
		start = month.Format("2006-01-02")
		end = month.AddDate(0, 1, -1).Format("2006-01-02")
	case "custom":
		first, err := time.Parse("2006-01-02", start)
		if err != nil || first.Format("2006-01-02") != start {
			return Totals{}, ErrInvalidInput
		}
		last, err := time.Parse("2006-01-02", end)
		if err != nil || last.Format("2006-01-02") != end || last.Before(first) {
			return Totals{}, ErrInvalidInput
		}
	default:
		return Totals{}, ErrInvalidInput
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.currency, e.direction, e.amount_minor, e.payment_method, m.display_name, e.category_id, COALESCE(c.name, ''), COALESCE(c.archived, FALSE) FROM transactions_entries e JOIN transactions_payment_methods m ON m.workspace_id = e.workspace_id AND m.name = e.payment_method LEFT JOIN transactions_expense_categories c ON c.workspace_id = e.workspace_id AND c.id = e.category_id WHERE e.workspace_id = $1 AND e.occurred_on >= $2 AND e.occurred_on <= $3 AND e.voided_at IS NULL ORDER BY e.currency, e.payment_method, e.category_id, e.id`, workspace, start, end)
	if err != nil {
		return Totals{}, err
	}
	defer rows.Close()
	type amounts struct{ in, out big.Int }
	type methodSum struct {
		item    MethodTotal
		amounts amounts
	}
	type categorySum struct {
		item CategoryTotal
		out  big.Int
	}
	type currencySum struct {
		item       CurrencyTotals
		amounts    amounts
		methods    map[string]*methodSum
		categories map[int64]*categorySum
	}
	sums := map[string]*currencySum{}
	for rows.Next() {
		var currency, direction, methodID, methodName, categoryName string
		var amount int64
		var categoryID sql.NullInt64
		var archived bool
		if err := rows.Scan(&currency, &direction, &amount, &methodID, &methodName, &categoryID, &categoryName, &archived); err != nil {
			return Totals{}, err
		}
		item := sums[currency]
		if item == nil {
			item = &currencySum{item: CurrencyTotals{Currency: currency}, methods: map[string]*methodSum{}, categories: map[int64]*categorySum{}}
			sums[currency] = item
		}
		method := item.methods[methodID]
		if method == nil {
			method = &methodSum{item: MethodTotal{ID: methodID, Name: methodName}}
			item.methods[methodID] = method
		}
		var value big.Int
		value.SetInt64(amount)
		if direction == "in" {
			item.amounts.in.Add(&item.amounts.in, &value)
			method.amounts.in.Add(&method.amounts.in, &value)
		} else {
			item.amounts.out.Add(&item.amounts.out, &value)
			method.amounts.out.Add(&method.amounts.out, &value)
			category := item.categories[categoryID.Int64]
			if category == nil {
				category = &categorySum{item: CategoryTotal{Name: categoryName, Archived: archived, Uncategorized: !categoryID.Valid}}
				if categoryID.Valid {
					id := CategoryID(categoryID.Int64)
					category.item.ID = &id
				}
				item.categories[categoryID.Int64] = category
			}
			category.out.Add(&category.out, &value)
		}
	}
	if err := rows.Err(); err != nil {
		return Totals{}, err
	}
	result := Totals{Period: period, Start: start, End: end, Currencies: []CurrencyTotals{}}
	for _, sum := range sums {
		item := sum.item
		item.MoneyIn, item.MoneyOut = sum.amounts.in.String(), sum.amounts.out.String()
		item.Net = new(big.Int).Sub(&sum.amounts.in, &sum.amounts.out).String()
		item.Methods = []MethodTotal{}
		item.Categories = []CategoryTotal{}
		for _, method := range sum.methods {
			method.item.MoneyIn, method.item.MoneyOut = method.amounts.in.String(), method.amounts.out.String()
			method.item.Net = new(big.Int).Sub(&method.amounts.in, &method.amounts.out).String()
			item.Methods = append(item.Methods, method.item)
		}
		for _, category := range sum.categories {
			category.item.MoneyOut = category.out.String()
			item.Categories = append(item.Categories, category.item)
		}
		sort.Slice(item.Methods, func(i, j int) bool { return item.Methods[i].ID < item.Methods[j].ID })
		sort.Slice(item.Categories, func(i, j int) bool {
			a, b := item.Categories[i], item.Categories[j]
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			if a.ID == nil {
				return true
			}
			if b.ID == nil {
				return false
			}
			return *a.ID < *b.ID
		})
		result.Currencies = append(result.Currencies, item)
	}
	sort.Slice(result.Currencies, func(i, j int) bool { return result.Currencies[i].Currency < result.Currencies[j].Currency })
	return result, nil
}
