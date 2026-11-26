package web

import (
	"io/fs"
	"testing"

	"example.com/dynamis-code/apps-template/internal/web/assets"
)

func TestBrowserAssetsExcludeUnreviewedContentTypes(t *testing.T) {
	appAssets, err := fs.Sub(files, "assets")
	if err != nil {
		t.Fatal(err)
	}
	for name, filesystem := range map[string]fs.FS{"app": appAssets, "preview": assets.Files} {
		t.Run(name, func(t *testing.T) {
			want := map[string]bool{
				"app.css": true, "clients.css": true, "components.css": true, "tokens.css": true,
				"alpine-components.js": true, "htmx.min.js": true, "js/alpine.min.js": true,
				"notes-editor.js": true, "notes-editor.css": true,
			}
			err := fs.WalkDir(filesystem, ".", func(name string, entry fs.DirEntry, err error) error {
				if err == nil && !entry.IsDir() {
					if !want[name] {
						t.Errorf("embedded unreviewed browser asset %q", name)
					}
					delete(want, name)
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(want) != 0 {
				t.Errorf("missing referenced browser assets: %v", want)
			}
		})
	}
}
