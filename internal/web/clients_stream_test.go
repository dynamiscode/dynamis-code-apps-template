package web

import (
	"context"
	"io"
	"testing"

	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/web/components/organisms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestClientListStreamsRows(t *testing.T) {
	assertListStreamsRows(t, clientListView)
}

func TestClientListPreservesRendering(t *testing.T) {
	assertListPreservesRendering(t, clientListView, func(data viewmodel.PageData) templ.Component {
		originalResults := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			limit := clientPageLimit(data.ClientQuery.Limit)
			if err := organisms.ListResults(data, limit, listPageSizeOptions(limit, listPageSizes), data.ClientQuerySuffix, clientBody(data), templ.NopComponent).Render(ctx, w); err != nil {
				return err
			}
			if !data.ListFragment {
				return nil
			}
			if _, err := io.WriteString(w, `<div id="client-list-scope" hx-swap-oob="true">`); err != nil {
				return err
			}
			if err := clientScope(data).Render(ctx, w); err != nil {
				return err
			}
			_, err := io.WriteString(w, `</div>`)
			return err
		})
		return organisms.ListScreen(data, "clients.search", clientListTools(data), originalResults)
	})
}
