// Package catalog owns the workspace catalog.
package catalog

import (
	"context"

	"example.com/dynamis-code/apps-template/internal/identity"
)

type Entry struct{}

type Service struct{ auth *identity.Service }

func NewService(auth *identity.Service) *Service { return &Service{auth: auth} }

func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID) ([]Entry, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, err
	}
	return []Entry{}, nil
}
