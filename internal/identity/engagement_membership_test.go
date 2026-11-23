package identity

import (
	"context"
	"database/sql"
	"testing"
)

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
			if _, err := db.Exec("INSERT INTO engagements (id, workspace_id, client_id, stage_id, assignee_id) VALUES (1, ?, 1, 1, ?)", boot.WorkspaceID, user.UserID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO engagement_next_actions (workspace_id, engagement_id, action_text, owner_id) VALUES (?, 1, 'Call', ?)", boot.WorkspaceID, user.UserID); err != nil {
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
			if err := db.QueryRow("SELECT e.assignee_id, a.owner_id, a.action_text FROM engagements e JOIN engagement_next_actions a ON a.workspace_id = e.workspace_id AND a.engagement_id = e.id WHERE e.workspace_id = ?", boot.WorkspaceID).Scan(&assignee, &actionOwner, &action); err != nil {
				t.Fatal(err)
			}
			if assignee.Valid || actionOwner.Valid || action != "Call" {
				t.Fatalf("references or action: %v %v %q", assignee, actionOwner, action)
			}
		})
	}
}
