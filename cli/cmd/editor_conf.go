package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shriyanss/report-rex/cli/internal/editorconf"
)

type editorConfFlagsT struct {
	IDE        string
	Host       string
	Port       int
	Username   string
	Password   string
	Bin        string
	Output     string
	Standalone bool
	NvimConfig string
	Dir        string
	Remove     bool
	CACert     string
	Insecure   bool
}

var editorConfFlags editorConfFlagsT

var editorConfCmd = &cobra.Command{
	Use:   "editor-conf",
	Short: "Generate an editor presence config (e.g. Neovim) for rex presence",
	RunE:  runEditorConf,
}

// defaultNvimConfigDir mirrors Neovim's own stdpath('config') resolution.
func defaultNvimConfigDir() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "nvim")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nvim")
}

func init() {
	f := editorConfCmd.Flags()
	f.StringVar(&editorConfFlags.IDE, "ide", "", "editor to generate a config for: nvim (required)")
	f.StringVar(&editorConfFlags.Host, "host", "", "sync server host (required)")
	f.IntVar(&editorConfFlags.Port, "port", 9443, "sync server port")
	f.StringVar(&editorConfFlags.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&editorConfFlags.Password, "password", "", "shared server password (omit and set $REX_PASSWORD instead to avoid plaintext in the generated file)")
	f.StringVar(&editorConfFlags.Bin, "bin", "", "rex binary to invoke from the editor (default: \"rex\" on $PATH)")
	f.StringVar(&editorConfFlags.Dir, "dir", ".", "report-rex workspace to keep synced (and, unless --standalone, to scope activation to)")
	f.BoolVar(&editorConfFlags.Standalone, "standalone", false, "generate a self-contained entrypoint (nvim -u ...) instead of installing into your existing Neovim config -- for a throwaway/test environment")
	f.StringVar(&editorConfFlags.NvimConfig, "nvim-config", "", "Neovim config directory to install into (default: $XDG_CONFIG_HOME/nvim or ~/.config/nvim); ignored with --standalone")
	f.StringVar(&editorConfFlags.Output, "output", ".", "directory to write the generated config into; only used with --standalone")
	f.BoolVar(&editorConfFlags.Remove, "remove", false, "remove a previously generated config instead of generating one (--host/--username not required)")
	f.StringVar(&editorConfFlags.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), baked into the generated config to trust it")
	f.BoolVar(&editorConfFlags.Insecure, "insecure", false, "bake --insecure (skip TLS certificate verification) into the generated config, as an alternative to --ca-cert")

	rootCmd.AddCommand(editorConfCmd)
}

func runEditorConf(cmd *cobra.Command, args []string) error {
	return runEditorConfWith(editorConfFlags)
}

func runEditorConfWith(f editorConfFlagsT) error {
	if f.IDE == "" {
		return fmt.Errorf("--ide is required (supported: nvim)")
	}
	if f.IDE != "nvim" {
		return fmt.Errorf("unknown --ide %q (supported: nvim)", f.IDE)
	}

	if f.Remove {
		return runEditorConfNvimRemove(f)
	}

	if f.Host == "" || f.Username == "" {
		return fmt.Errorf("--host and --username are required")
	}
	return runEditorConfNvim(f)
}

func runEditorConfNvimRemove(f editorConfFlagsT) error {
	if f.Standalone {
		if err := editorconf.RemoveNvimStandalone(f.Output); err != nil {
			return err
		}
		fmt.Printf("Removed %s/rex-presence-nvim/ and %s/rex-presence-init.lua\n", f.Output, f.Output)
		return nil
	}

	nvimConfig := f.NvimConfig
	if nvimConfig == "" {
		nvimConfig = defaultNvimConfigDir()
	}
	if err := editorconf.RemoveNvimLazyPlugin(nvimConfig); err != nil {
		return err
	}
	fmt.Printf("Removed %s/rex-presence-nvim/ and %s/lua/plugins/rex-presence.lua\n", nvimConfig, nvimConfig)
	fmt.Printf("Restart Neovim (or run :Lazy sync) for the removal to take effect.\n")
	return nil
}

func runEditorConfNvim(f editorConfFlagsT) error {
	dir, err := filepath.Abs(f.Dir)
	if err != nil {
		return fmt.Errorf("resolving --dir: %w", err)
	}
	// Resolve symlinks (e.g. macOS's /tmp -> /private/tmp) so this matches
	// what Neovim's vim.fn.getcwd() reports -- it always resolves them, and
	// a mismatch here would make the scoped-activation check never match.
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	opts := editorconf.NvimOptions{
		Host: f.Host, Port: f.Port, Username: f.Username,
		Password: f.Password, Bin: f.Bin, Dir: dir,
		CACert: f.CACert, Insecure: f.Insecure,
	}

	if f.Standalone {
		path, err := editorconf.GenerateNvimStandalone(f.Output, opts)
		if err != nil {
			return err
		}
		fmt.Printf("Generated Neovim presence config:\n")
		fmt.Printf("  %s/rex-presence-nvim/ (plugin source)\n", f.Output)
		fmt.Printf("  %s (entrypoint)\n\n", path)
		fmt.Printf("Launch with:\n  nvim -u %s <file>\n", path)
		if f.Password == "" {
			fmt.Printf("\nNote: export REX_PASSWORD before launching nvim (no --password was given).\n")
		}
		return nil
	}

	nvimConfig := f.NvimConfig
	if nvimConfig == "" {
		nvimConfig = defaultNvimConfigDir()
	}
	path, err := editorconf.GenerateNvimLazyPlugin(nvimConfig, opts)
	if err != nil {
		return err
	}
	fmt.Printf("Generated Neovim presence plugin:\n")
	fmt.Printf("  %s/rex-presence-nvim/ (plugin source)\n", nvimConfig)
	fmt.Printf("  %s (lazy.nvim plugin spec)\n\n", path)
	fmt.Printf("Restart Neovim (or run :Lazy sync) to load it. It only activates when\nNeovim is opened inside %s.\n", dir)
	fmt.Printf("\nAdd require(\"rex-presence\").statusline() to your own lualine config to show remote presence there too.\n")
	if f.Password == "" {
		fmt.Printf("\nNote: export REX_PASSWORD before launching nvim (no --password was given).\n")
	}
	return nil
}
