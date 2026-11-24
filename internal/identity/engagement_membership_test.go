package identity

import (
	"context"
	"database/sql"
	"testing"
)

func TestAccountDeletionAdvancesActionVersion(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	boot := bootstrapOwner(t, service)
	user := insertUser(t, service, db, "delete-action-owner@example.com")
	insertMembership(t, service, db, boot.WorkspaceID, user, Member)
	for _, query := range []string{
		"INSERT INTO clients (id, workspace_id, display_name, version, created_at, updated_at) VALUES (1, ?, 'Client', 1, '', '')",
		"INSERT INTO engagement_stages (id, workspace_id, name, kind, position) VALUES (1, ?, 'Open', 'open', 1)",
		"INSERT INTO engagements (id, workspace_id, client_id, stage_id, action_version) VALUES (1, ?, 1, 1, 1)",
	} {
		if _, err := db.Exec(query, boot.WorkspaceID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO engagement_next_actions (workspace_id, engagement_id, action_text, owner_id) VALUES (?, 1, 'Call', ?)", boot.WorkspaceID, user); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteAccount(ctx, user, "user-long-password", AuditContext{}); err != nil {
		t.Fatal(err)
	}
	var owner sql.NullInt64
	var version, counter int64
	if err := db.QueryRow("SELECT a.owner_id, a.version, e.action_version FROM engagement_next_actions a JOIN engagements e ON e.workspace_id = a.workspace_id AND e.id = a.engagement_id WHERE a.workspace_id = ?", boot.WorkspaceID).Scan(&owner, &version, &counter); err != nil || owner.Valid || version != 2 || counter != version {
		t.Fatalf("owner = %v, version = %d, counter = %d, error = %v", owner, version, counter, err)
	}
}

func TestMembershipRemovalClearsEngagementReferences(t *testing.T) {
	for _, method := range []string{"member", "scim-user", "scim-group"} {
		t.Run(method, func(t *testing.T) {
			service, db := newTestService(t)
			ctx := context.Background()
			boot := bootstrapOwner(t, service)
			owner := mustAuthorize(t, service, boot.UserID, boot.WorkspaceID, SCIMManage)
			token, err := service.CreateSCIMToken(ctx, owner, AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
			provisioner, err := service.AuthenticateSCIMToken(ctx, token.Secret, boot.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			user, err := service.CreateSCIMUser(ctx, provisioner, SCIMUserInput{UserName: "member@example.com"}, AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
			for _, query := range []string{
				"INSERT INTO clients (id, workspace_id, display_name, version, created_at, updated_at) VALUES (1, ?, 'Client', 1, '', '')",
				"INSERT INTO engagement_stages (id, workspace_id, name, kind, position) VALUES (1, ?, 'Open', 'open', 1)",
			} {
				if _, err := db.Exec(query, boot.WorkspaceID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec("INSERT INTO engagements (id, workspace_id, client_id, stage_id, assignee_id, action_version) VALUES (1, ?, 1, 1, ?, 1)", boot.WorkspaceID, user.UserID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO engagement_next_actions (workspace_id, engagement_id, action_text, owner_id) VALUES (?, 1, 'Call', ?)", boot.WorkspaceID, user.UserID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO engagement_action_history (workspace_id, engagement_id, action_text, owner_id, actor_user_id, outcome, created_at) VALUES (?, 1, 'Previous', ?, ?, 'replaced', '2020-01-01T00:00:00Z')", boot.WorkspaceID, user.UserID, user.UserID); err != nil {
				t.Fatal(err)
			}
			switch method {
			case "member":
				err = service.RemoveMember(ctx, owner, memberIDFor(t, service, owner, user.UserID), AuditContext{})
			case "scim-user":
				_, err = service.PatchSCIMUser(ctx, provisioner, user.ID, SCIMUserPatch{Active: boolPtr(false)}, user.Version, AuditContext{})
			case "scim-group":
				groups, listErr := service.ListSCIMGroups(ctx, provisioner)
				if listErr != nil {
					t.Fatal(listErr)
				}
				for _, group := range groups {
					if group.DisplayName == "member" {
						_, err = service.PatchSCIMGroup(ctx, provisioner, "member", []SCIMGroupOperation{{Operation: "remove", Members: []string{user.ID}}}, group.Version, AuditContext{})
						break
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			var assignee, actionOwner sql.NullInt64
			var action string
			var version, counter int64
			if err := db.QueryRow("SELECT e.assignee_id, a.owner_id, a.action_text, a.version, e.action_version FROM engagements e JOIN engagement_next_actions a ON a.workspace_id = e.workspace_id AND a.engagement_id = e.id WHERE e.workspace_id = ?", boot.WorkspaceID).Scan(&assignee, &actionOwner, &action, &version, &counter); err != nil {
				t.Fatal(err)
			}
			if assignee.Valid || actionOwner.Valid || action != "Call" || version != 2 || counter != version {
				t.Fatalf("references, action or version: %v %v %q %d %d", assignee, actionOwner, action, version, counter)
			}
			var historyOwner, historyActor sql.NullInt64
			if err := db.QueryRow("SELECT owner_id, actor_user_id, action_text FROM engagement_action_history WHERE workspace_id = ?", boot.WorkspaceID).Scan(&historyOwner, &historyActor, &action); err != nil {
				t.Fatal(err)
			}
			if historyOwner.Valid || historyActor.Valid || action != "Previous" {
				t.Fatalf("history references or action: %v %v %q", historyOwner, historyActor, action)
			}
		})
	}
}
