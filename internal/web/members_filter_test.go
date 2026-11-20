package web

import (
	"net/http"
	"strings"
	"testing"
)

// TestMembersListFiltersUseSharedConditions checks the members filters through the shared model: a role
// condition, the match mode across two conditions, the legacy role link, invalid input, and the add step.
func TestMembersListFiltersUseSharedConditions(t *testing.T) {
	env := newMembersListTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/settings/members"
	list := func(t *testing.T, query string) (int, string) {
		t.Helper()
		response := request(env.routes, http.MethodGet, path+query, nil, env.cookies, nil)
		return response.Code, response.Body.String()
	}
	// The one member is the owner, so the owner condition keeps it and the admin condition removes it.
	if code, body := list(t, "?filter=role:equals:owner"); code != http.StatusOK || !strings.Contains(body, "1 member") {
		t.Fatalf("owner condition = %d, want 1 member: %s", code, body)
	}
	if code, body := list(t, "?filter=role:equals:admin"); code != http.StatusOK || !strings.Contains(body, "0 members") {
		t.Fatalf("admin condition = %d, want 0 members: %s", code, body)
	}
	// Any keeps the member when one condition holds; all needs both to hold.
	if code, body := list(t, "?filter=role:equals:owner&filter=email:contains:zzz-no-match&match=any"); code != http.StatusOK || !strings.Contains(body, "1 member") {
		t.Fatalf("match any = %d, want 1 member: %s", code, body)
	}
	if code, body := list(t, "?filter=role:equals:owner&filter=email:contains:zzz-no-match&match=all"); code != http.StatusOK || !strings.Contains(body, "0 members") {
		t.Fatalf("match all = %d, want 0 members: %s", code, body)
	}
	// The older role link still works, as a condition.
	if code, body := list(t, "?role=admin"); code != http.StatusOK || !strings.Contains(body, "0 members") {
		t.Fatalf("legacy role link = %d, want 0 members: %s", code, body)
	}
	// A role outside the choices, and a match mode that is not all or any, are refused.
	if code, _ := list(t, "?filter=role:equals:superuser"); code != http.StatusBadRequest {
		t.Fatalf("unknown role = %d, want 400", code)
	}
	if code, _ := list(t, "?filter=role:equals:owner&match=sometimes"); code != http.StatusBadRequest {
		t.Fatalf("unknown match = %d, want 400", code)
	}
	// Choosing a field without a value opens its condition step, not a filter.
	if code, body := list(t, "?filter_field=role"); code != http.StatusOK || !strings.Contains(body, `name="filter_op"`) ||
		!strings.Contains(body, `name="filter_field" value="role"`) {
		t.Fatalf("add step for role = %d: %s", code, body)
	}
}
