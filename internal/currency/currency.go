package currency

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	cldr "golang.org/x/text/currency"
)

var ErrInvalidInput = errors.New("invalid currency configuration")
var ErrInUse = errors.New("currency is in use")

type Currency struct {
	Code     string
	Decimals int
	Default  bool
}

// availableCodes is the supported ISO 4217 selection. The pinned x/text CLDR
// tables supply standard precision; newer ISO codes are explicit.
const availableCodes = "AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BRL BSD BTN BWP BYN BZD CAD CDF CHF CLP CNY COP CRC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SOS SRD SSP STN SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD UYU UZS VED VES VND VUV WST XAF XCD XOF XPF YER ZAR ZMW ZWG"

func Available() []Currency {
	codes := strings.Fields(availableCodes)
	out := make([]Currency, 0, len(codes))
	for _, code := range codes {
		c, _ := Canonical(code)
		out = append(out, c)
	}
	return out
}

// Canonical returns immutable standard precision for supported ISO codes.
func Canonical(code string) (Currency, bool) {
	if len(code) != 3 || !strings.Contains(availableCodes, code) {
		return Currency{}, false
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return Currency{}, false
		}
	}
	decimals := 2
	switch code {
	case "MRU", "SLE", "VED", "VES", "ZWG":
	default:
		if unit, err := cldr.ParseISO(code); err == nil {
			decimals, _ = cldr.Standard.Rounding(unit)
		}
	}
	return Currency{Code: code, Decimals: decimals}, true
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// List reads workspace configuration, including the implicit USD for new workspaces.
func List(ctx context.Context, db querier, workspaceID identity.WorkspaceID) ([]Currency, error) {
	rows, err := db.QueryContext(ctx, `SELECT code, decimals, is_default FROM workspace_currencies WHERE workspace_id = $1 ORDER BY is_default DESC, code`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Currency
	for rows.Next() {
		var c Currency
		if err := rows.Scan(&c.Code, &c.Decimals, &c.Default); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return []Currency{{Code: "USD", Decimals: 2, Default: true}}, nil
	}
	return result, nil
}

type Service struct {
	db   *sql.DB
	auth *identity.Service
}

func NewService(db *sql.DB, auth *identity.Service) *Service { return &Service{db: db, auth: auth} }

func (s *Service) ListCurrencies(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID) ([]Currency, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, err
	}
	return List(ctx, s.db, workspaceID)
}

func (s *Service) SetCurrencies(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, currencies []Currency) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite)
	if err != nil {
		return err
	}
	if principal.Role != identity.Owner && principal.Role != identity.Admin {
		return identity.ErrForbidden
	}
	// Lock the workspace before reading the current configuration.
	if _, err := tx.ExecContext(ctx, `UPDATE workspaces SET id = id WHERE id = $1`, workspaceID); err != nil {
		return err
	}
	previous, err := List(ctx, tx, workspaceID)
	if err != nil {
		return err
	}
	old := make(map[string]Currency, len(previous))
	for _, c := range previous {
		old[c.Code] = c
	}
	allowed := make(map[string]int)
	for _, c := range Available() {
		allowed[c.Code] = c.Decimals
	}
	seen := make(map[string]bool, len(currencies))
	defaults := 0
	for _, c := range currencies {
		if c.Code != strings.TrimSpace(c.Code) || seen[c.Code] || c.Decimals < 0 || c.Decimals > 18 {
			return ErrInvalidInput
		}
		if existing, ok := old[c.Code]; ok {
			if c.Decimals != existing.Decimals {
				return ErrInvalidInput
			}
		} else if decimals, ok := allowed[c.Code]; !ok || c.Decimals != decimals {
			return ErrInvalidInput
		}
		seen[c.Code] = true
		if c.Default {
			if canonical, ok := Canonical(c.Code); (!ok || canonical.Decimals != c.Decimals) && !old[c.Code].Default {
				return ErrInvalidInput
			}
			defaults++
		}
	}
	if defaults != 1 {
		return ErrInvalidInput
	}
	var hasCatalog bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 45)`).Scan(&hasCatalog); err != nil {
		return err
	}
	if hasCatalog {
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT p.currency FROM catalog_prices p JOIN catalog_entries e ON e.id = p.entry_id WHERE e.workspace_id = $1`, workspaceID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var code string
			if err := rows.Scan(&code); err != nil {
				rows.Close()
				return err
			}
			if !seen[code] {
				rows.Close()
				return ErrInUse
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_currencies WHERE workspace_id = $1`, workspaceID); err != nil {
		return err
	}
	for _, c := range currencies {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_currencies (workspace_id, code, decimals, is_default) VALUES ($1, $2, $3, $4)`, workspaceID, c.Code, c.Decimals, c.Default); err != nil {
			return err
		}
	}
	return tx.Commit()
}
