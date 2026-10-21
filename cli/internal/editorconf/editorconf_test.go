package editorconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateNvimStandalone(t *testing.T) {
	dir := t.TempDir()

	path, err := GenerateNvimStandalone(dir, NvimOptions{
		Host:     "rex-svr.lan",
		Port:     9443,
		Username: "alice",
		Dir:      "/home/alice/report-1",
	})
	if err != nil {
		t.Fatalf("GenerateNvimStandalone: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "rex-presence-nvim", "lua", "rex-presence", "init.lua")); err != nil {
		t.Errorf("plugin not extracted: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	got := string(content)

	for _, want := range []string{
		`host = "rex-svr.lan"`,
		`port = 9443`,
		`username = "alice"`,
		`bin = "rex"`,
		`dir = "/home/alice/report-1"`,
		"rtp:prepend",
		"export $REX_PASSWORD",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated entrypoint missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "password =") {
		t.Errorf("password field should be omitted when no password given:\n%s", got)
	}
	if strings.Contains(got, "scoped") {
		t.Errorf("standalone entrypoint should not be scoped:\n%s", got)
	}
}

func TestGenerateNvimStandaloneWithPassword(t *testing.T) {
	dir := t.TempDir()
	path, err := GenerateNvimStandalone(dir, NvimOptions{
		Host:     "127.0.0.1",
		Username: "bob",
		Password: "abc123",
		Bin:      "/opt/rex",
		Dir:      "/home/bob/report-2",
	})
	if err != nil {
		t.Fatalf("GenerateNvimStandalone: %v", err)
	}
	content, _ := os.ReadFile(path)
	got := string(content)
	if !strings.Contains(got, `password = "abc123"`) {
		t.Errorf("expected password to be baked in, got:\n%s", got)
	}
	if !strings.Contains(got, `bin = "/opt/rex"`) {
		t.Errorf("expected custom bin path, got:\n%s", got)
	}
}

func TestGenerateNvimLazyPlugin(t *testing.T) {
	nvimConfig := t.TempDir()

	path, err := GenerateNvimLazyPlugin(nvimConfig, NvimOptions{
		Host:     "rex-svr.lan",
		Port:     9443,
		Username: "alice",
		Dir:      "/home/alice/report-1",
	})
	if err != nil {
		t.Fatalf("GenerateNvimLazyPlugin: %v", err)
	}

	if want := filepath.Join(nvimConfig, "lua", "plugins", "rex-presence.lua"); path != want {
		t.Errorf("path = %s, want %s", path, want)
	}
	if _, err := os.Stat(filepath.Join(nvimConfig, "rex-presence-nvim", "lua", "rex-presence", "init.lua")); err != nil {
		t.Errorf("plugin not extracted: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	got := string(content)

	for _, want := range []string{
		`host = "rex-svr.lan"`,
		`username = "alice"`,
		`dir = "/home/alice/report-1"`,
		"scoped = true",
		"lazy = false",
		`name = "rex-presence"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated spec missing %q:\n%s", want, got)
		}
	}
}

func TestRemoveNvimStandalone(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateNvimStandalone(dir, NvimOptions{Host: "h", Username: "u", Dir: "/w"}); err != nil {
		t.Fatalf("GenerateNvimStandalone: %v", err)
	}
	if err := RemoveNvimStandalone(dir); err != nil {
		t.Fatalf("RemoveNvimStandalone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rex-presence-init.lua")); !os.IsNotExist(err) {
		t.Errorf("entrypoint should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rex-presence-nvim")); !os.IsNotExist(err) {
		t.Errorf("plugin dir should be removed, stat err = %v", err)
	}
}

func TestRemoveNvimStandaloneIsSafeWhenNothingExists(t *testing.T) {
	if err := RemoveNvimStandalone(t.TempDir()); err != nil {
		t.Fatalf("RemoveNvimStandalone on empty dir: %v", err)
	}
}

func TestRemoveNvimLazyPlugin(t *testing.T) {
	nvimConfig := t.TempDir()
	if _, err := GenerateNvimLazyPlugin(nvimConfig, NvimOptions{Host: "h", Username: "u", Dir: "/w"}); err != nil {
		t.Fatalf("GenerateNvimLazyPlugin: %v", err)
	}
	if err := RemoveNvimLazyPlugin(nvimConfig); err != nil {
		t.Fatalf("RemoveNvimLazyPlugin: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nvimConfig, "lua", "plugins", "rex-presence.lua")); !os.IsNotExist(err) {
		t.Errorf("spec should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(nvimConfig, "rex-presence-nvim")); !os.IsNotExist(err) {
		t.Errorf("plugin dir should be removed, stat err = %v", err)
	}
}

func TestRemoveNvimLazyPluginIsSafeWhenNothingExists(t *testing.T) {
	if err := RemoveNvimLazyPlugin(t.TempDir()); err != nil {
		t.Fatalf("RemoveNvimLazyPlugin on empty dir: %v", err)
	}
}
