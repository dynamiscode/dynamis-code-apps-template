package database

import "example.com/dynamis-code/apps-template/internal/platform/config"

// The helpers below give one query shape per dialect for the operations that
// differ between SQLite and PostgreSQL. Callers pass SQL fragments and
// placeholders (?), and Rebind numbers them for PostgreSQL.

const (
	upperASCII = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowerASCII = "abcdefghijklmnopqrstuvwxyz"
)

// ASCIILowerExpr folds ASCII letters to lower case. Other characters are left
// as they are. SQLite lower() is ASCII-only without ICU, so PostgreSQL uses
// translate() to match it. lower() there depends on the database locale.
func ASCIILowerExpr(driver config.DatabaseDriver, expr string) string {
	if driver == config.Postgres {
		return "translate(" + expr + ", '" + upperASCII + "', '" + lowerASCII + "')"
	}
	return "lower(" + expr + ")"
}

// ContainsExpr is a boolean expression that is true when haystack contains
// needle. It compares exactly, so callers fold both sides first.
func ContainsExpr(driver config.DatabaseDriver, haystack, needle string) string {
	if driver == config.Postgres {
		return "strpos(" + haystack + ", " + needle + ") > 0"
	}
	return "instr(" + haystack + ", " + needle + ") > 0"
}

// NumericExpr casts text to a number for ordering and comparing number fields.
// ponytail: the cast is not indexed. A numeric slot index needs a migration, so
// ordering and range filters on number fields scan the workspace's rows. Add
// typed numeric indexes when a measured workspace needs them.
func NumericExpr(driver config.DatabaseDriver, expr string) string {
	if driver == config.Postgres {
		return "(" + expr + ")::numeric"
	}
	return "CAST(" + expr + " AS REAL)"
}

// TextOrderExpr orders text by its bytes. PostgreSQL's default collation sorts
// by locale, which differs from SQLite's binary order, so PostgreSQL uses the
// "C" collation.
func TextOrderExpr(driver config.DatabaseDriver, expr string) string {
	if driver == config.Postgres {
		return "(" + expr + ") COLLATE \"C\""
	}
	return expr
}
