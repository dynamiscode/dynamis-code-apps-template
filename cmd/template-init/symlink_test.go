package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyTemplateKeepsInTreeSymlinkAndRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.agents/skills", filepath.Join(root, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "app")
	if err := copyTemplate(root, output, nil, "app", "app", true, true); err != nil {
		t.Fatalf("copyTemplate() error = %v", err)
	}
	link, err := os.Readlink(filepath.Join(output, ".claude", "skills"))
	if err != nil || link != "../.agents/skills" {
		t.Fatalf("generated skills link = %q, error = %v; want ../.agents/skills", link, err)
	}

	escaping := t.TempDir()
	if err := os.Symlink("../../outside", filepath.Join(escaping, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := copyTemplate(escaping, filepath.Join(t.TempDir(), "app"), nil, "app", "app", true, true); err == nil {
		t.Fatal("copyTemplate() accepted a symlink that escapes the template")
	}
}
