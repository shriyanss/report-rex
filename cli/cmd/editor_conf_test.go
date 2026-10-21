package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEditorConfRequiresHostAndUsername(t *testing.T) {
	err := runEditorConfWith(editorConfFlagsT{IDE: "nvim", Username: "alice"})
	if err == nil || !strings.Contains(err.Error(), "--host") {
		t.Fatalf("expected a --host error, got %v", err)
	}

	err = runEditorConfWith(editorConfFlagsT{IDE: "nvim", Host: "rex-svr.lan"})
	if err == nil || !strings.Contains(err.Error(), "--username") {
		t.Fatalf("expected a --username error, got %v", err)
	}
}

func TestRunEditorConfRequiresIDE(t *testing.T) {
	err := runEditorConfWith(editorConfFlagsT{Host: "rex-svr.lan", Username: "alice"})
	if err == nil || !strings.Contains(err.Error(), "--ide") {
		t.Fatalf("expected an --ide error, got %v", err)
	}
}

func TestRunEditorConfRejectsUnknownIDE(t *testing.T) {
	err := runEditorConfWith(editorConfFlagsT{IDE: "vscode", Host: "rex-svr.lan", Username: "alice"})
	if err == nil || !strings.Contains(err.Error(), "vscode") {
		t.Fatalf("expected an unknown-ide error, got %v", err)
	}
}

func TestRunEditorConfGeneratesStandaloneNvimConfig(t *testing.T) {
	dir := t.TempDir()
	err := runEditorConfWith(editorConfFlagsT{
		IDE: "nvim", Host: "rex-svr.lan", Port: 9443, Username: "alice", Output: dir, Standalone: true,
	})
	if err != nil {
		t.Fatalf("runEditorConfWith: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rex-presence-init.lua")); err != nil {
		t.Errorf("expected generated entrypoint: %v", err)
	}
}

func TestRunEditorConfGeneratesLazyPluginByDefault(t *testing.T) {
	nvimConfig := t.TempDir()
	err := runEditorConfWith(editorConfFlagsT{
		IDE: "nvim", Host: "rex-svr.lan", Port: 9443, Username: "alice", NvimConfig: nvimConfig,
	})
	if err != nil {
		t.Fatalf("runEditorConfWith: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nvimConfig, "lua", "plugins", "rex-presence.lua")); err != nil {
		t.Errorf("expected generated plugin spec: %v", err)
	}
}

func TestRunEditorConfRemoveDoesNotRequireHostOrUsername(t *testing.T) {
	nvimConfig := t.TempDir()
	if err := runEditorConfWith(editorConfFlagsT{IDE: "nvim", NvimConfig: nvimConfig, Remove: true}); err != nil {
		t.Fatalf("runEditorConfWith(remove): %v", err)
	}
}

func TestRunEditorConfRemoveLazyPlugin(t *testing.T) {
	nvimConfig := t.TempDir()
	if err := runEditorConfWith(editorConfFlagsT{
		IDE: "nvim", Host: "rex-svr.lan", Username: "alice", NvimConfig: nvimConfig,
	}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	spec := filepath.Join(nvimConfig, "lua", "plugins", "rex-presence.lua")
	if _, err := os.Stat(spec); err != nil {
		t.Fatalf("expected spec to exist before removal: %v", err)
	}

	if err := runEditorConfWith(editorConfFlagsT{IDE: "nvim", NvimConfig: nvimConfig, Remove: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(spec); !os.IsNotExist(err) {
		t.Errorf("expected spec to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(nvimConfig, "rex-presence-nvim")); !os.IsNotExist(err) {
		t.Errorf("expected vendored plugin dir to be removed, stat err = %v", err)
	}
}

func TestRunEditorConfRemoveStandalone(t *testing.T) {
	dir := t.TempDir()
	if err := runEditorConfWith(editorConfFlagsT{
		IDE: "nvim", Host: "rex-svr.lan", Username: "alice", Output: dir, Standalone: true,
	}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	entrypoint := filepath.Join(dir, "rex-presence-init.lua")
	if _, err := os.Stat(entrypoint); err != nil {
		t.Fatalf("expected entrypoint to exist before removal: %v", err)
	}

	if err := runEditorConfWith(editorConfFlagsT{IDE: "nvim", Output: dir, Standalone: true, Remove: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(entrypoint); !os.IsNotExist(err) {
		t.Errorf("expected entrypoint to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rex-presence-nvim")); !os.IsNotExist(err) {
		t.Errorf("expected vendored plugin dir to be removed, stat err = %v", err)
	}
}
