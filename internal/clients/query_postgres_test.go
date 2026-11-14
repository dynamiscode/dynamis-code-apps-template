package clients

import "testing"

// TestPostgresClientQueries runs the same query rules as the SQLite test on
// PostgreSQL. It is skipped unless POSTGRES_TEST_URL is set.
func TestPostgresClientQueries(t *testing.T) {
	exerciseClientQueries(t, newPostgresFixture(t))
}
