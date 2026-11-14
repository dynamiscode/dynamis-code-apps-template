package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// checkDialectExprs runs the shared expression helpers on one dialect. Both
// dialects must give the same answers, so the expected values are identical.
func checkDialectExprs(t *testing.T, driver config.DatabaseDriver, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	scalars := []struct {
		name, expr, want string
	}{
		{"ASCII fold lowers ASCII only", ASCIILowerExpr(driver, "'Ünï ABC'"), "Ünï abc"},
		{"contains on folded text", fmt.Sprintf("CASE WHEN %s THEN 'yes' ELSE 'no' END",
			ContainsExpr(driver, ASCIILowerExpr(driver, "'Analytical'"), "'alyt'")), "yes"},
		{"contains is exact for non-ASCII", fmt.Sprintf("CASE WHEN %s THEN 'yes' ELSE 'no' END",
			ContainsExpr(driver, ASCIILowerExpr(driver, "'Ünï'"), "'ünï'")), "no"},
		{"numeric compare is numeric", fmt.Sprintf("CASE WHEN %s < %s THEN 'yes' ELSE 'no' END",
			NumericExpr(driver, "'9'"), NumericExpr(driver, "'10'")), "yes"},
	}
	for _, tc := range scalars {
		var got string
		if err := db.QueryRowContext(ctx, Rebind(driver, "SELECT "+tc.expr)).Scan(&got); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}

	// Binary order: uppercase sorts before lowercase, on both dialects.
	rows, err := db.QueryContext(ctx, "SELECT v FROM (SELECT 'a' AS v UNION ALL SELECT 'B' UNION ALL SELECT 'b') AS s ORDER BY "+TextOrderExpr(driver, "v"))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var order []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		order = append(order, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"B", "a", "b"}; !slices.Equal(order, want) {
		t.Fatalf("text order = %v, want %v", order, want)
	}
}

func TestSQLiteDialectExprs(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "expr.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	checkDialectExprs(t, config.SQLite, db)
}

func TestPostgresDialectExprs(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	db, err := Open(context.Background(), config.Database{
		Driver: config.Postgres, URL: url, MaxOpenConns: 4, MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	checkDialectExprs(t, config.Postgres, db)
}
