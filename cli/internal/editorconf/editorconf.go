// Package editorconf generates ready-to-use editor presence configs for
// `rex editor-conf`, extracting the matching embedded editor integration
// source and writing a small file that wires it up.
package editorconf

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/shriyanss/report-rex/cli/internal/embedded"
	"github.com/shriyanss/report-rex/cli/internal/fsutil"
)

// NvimOptions configures the generated Neovim integration.
type NvimOptions struct {
	Host     string
	Port     int
	Username string
	Password string // if empty, relies on $REX_PASSWORD instead
	Bin      string // if empty, "rex" is assumed to be on $PATH
	Dir      string // absolute path to the report-rex workspace to keep synced
	CACert   string // path to rex-svr's self-signed certificate, to trust it
	Insecure bool   // skip TLS certificate verification instead of trusting CACert
}

// extractNvimPlugin extracts the embedded rex-presence.nvim plugin into
// <destDir>/rex-presence-nvim/, returning its absolute path.
func extractNvimPlugin(destDir string) (string, error) {
	src, err := fs.Sub(embedded.EditorsFS(), "nvim")
	if err != nil {
		return "", fmt.Errorf("embedded nvim plugin: %w", err)
	}
	pluginDir := filepath.Join(destDir, "rex-presence-nvim")
	if err := fsutil.CopyFS(src, pluginDir); err != nil {
		return "", fmt.Errorf("extracting nvim plugin: %w", err)
	}
	return filepath.Abs(pluginDir)
}

// GenerateNvimStandalone extracts the embedded rex-presence.nvim plugin into
// <outputDir>/rex-presence-nvim/ and writes a self-contained
// <outputDir>/rex-presence-init.lua entrypoint (`nvim -u <that file>`),
// returning the entrypoint's absolute path. This bypasses whatever Neovim
// config already exists -- the right tool for a throwaway/test environment,
// not a daily driver.
func GenerateNvimStandalone(outputDir string, opts NvimOptions) (string, error) {
	pluginDir, err := extractNvimPlugin(outputDir)
	if err != nil {
		return "", err
	}

	entrypoint := filepath.Join(outputDir, "rex-presence-init.lua")
	if err := writeStandaloneEntrypoint(entrypoint, pluginDir, opts); err != nil {
		return "", err
	}
	return filepath.Abs(entrypoint)
}

// GenerateNvimLazyPlugin extracts the embedded rex-presence.nvim plugin into
// <nvimConfigDir>/rex-presence-nvim/ and writes a lazy.nvim plugin spec to
// <nvimConfigDir>/lua/plugins/rex-presence.lua -- the standard "drop a spec
// file under lua/plugins/" convention LazyVim and plain lazy.nvim configs
// following lazy.nvim's own documented layout both auto-import, so it loads
// alongside the user's existing plugins/keymaps/colorscheme rather than
// replacing them. Returns the spec file's absolute path. Both extraction and
// the spec file use fixed names, so re-running this safely regenerates/
// overwrites a prior config rather than accumulating stale files.
func GenerateNvimLazyPlugin(nvimConfigDir string, opts NvimOptions) (string, error) {
	pluginDir, err := extractNvimPlugin(nvimConfigDir)
	if err != nil {
		return "", err
	}

	specDir := filepath.Join(nvimConfigDir, "lua", "plugins")
	spec := filepath.Join(specDir, "rex-presence.lua")
	if err := writeLazyPluginSpec(spec, pluginDir, opts); err != nil {
		return "", err
	}
	return filepath.Abs(spec)
}

// RemoveNvimStandalone deletes the files GenerateNvimStandalone writes
// (safe to call even if they don't exist).
func RemoveNvimStandalone(outputDir string) error {
	return removeAll(
		filepath.Join(outputDir, "rex-presence-nvim"),
		filepath.Join(outputDir, "rex-presence-init.lua"),
	)
}

// RemoveNvimLazyPlugin deletes the files GenerateNvimLazyPlugin writes
// (safe to call even if they don't exist).
func RemoveNvimLazyPlugin(nvimConfigDir string) error {
	return removeAll(
		filepath.Join(nvimConfigDir, "rex-presence-nvim"),
		filepath.Join(nvimConfigDir, "lua", "plugins", "rex-presence.lua"),
	)
}

func removeAll(paths ...string) error {
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("removing %s: %w", p, err)
		}
	}
	return nil
}
