import * as vscode from "vscode";
import { ProjectOverride } from "./lint";

// passwordSecretKey namespaces the SecretStorage key by workspace folder --
// SecretStorage itself is extension-global, so without this two workspaces
// would share one password. Matches every other report-rex.* setting, which
// is already written with ConfigurationTarget.Workspace.
export function passwordSecretKey(): string {
  const folder = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ?? "";
  return `report-rex.password::${folder}`;
}

export interface RexConfig {
  host: string;
  port: number;
  username: string;
  dir: string;
  bin: string;
  verbose: boolean;
  syncEnabled: boolean;
  password?: string;
  caCert: string;
  insecure: boolean;
}

// resolveConfig reads report-rex.* workspace settings plus the SecretStorage
// password. Returns null if host/username aren't set yet -- callers treat
// that as "not configured", not an error (fail open, never block editing).
export async function resolveConfig(context: vscode.ExtensionContext): Promise<RexConfig | null> {
  const cfg = vscode.workspace.getConfiguration("report-rex");
  const host = cfg.get<string>("host", "");
  const username = cfg.get<string>("username", "");
  if (!host || !username) {
    return null;
  }
  const folder = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ?? "";
  const dir = cfg.get<string>("dir", "") || folder;
  // $REX_PASSWORD (what the CLI itself falls back to) covers setups that
  // keep the password out of VS Code entirely, e.g. a launched test host.
  const password = (await context.secrets.get(passwordSecretKey())) || process.env.REX_PASSWORD || undefined;
  return {
    host,
    port: cfg.get<number>("port", 9443),
    username,
    dir,
    bin: cfg.get<string>("bin", "rex"),
    verbose: cfg.get<boolean>("verbose", false),
    syncEnabled: cfg.get<boolean>("sync.enabled", false),
    password,
    caCert: cfg.get<string>("caCert", ""),
    insecure: cfg.get<boolean>("insecure", false),
  };
}

export interface LintConfig {
  dir: string;
  bin: string;
  enabled: boolean;
  projectOverride: ProjectOverride;
}

// resolveLintConfig reads only report-rex.dir/bin plus the lint-specific
// settings -- deliberately independent of resolveConfig()'s host/username
// requirement, since linting must work standalone, without any rex-svr
// presence/sync setup.
export function resolveLintConfig(): LintConfig {
  const cfg = vscode.workspace.getConfiguration("report-rex");
  const folder = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ?? "";
  return {
    dir: cfg.get<string>("dir", "") || folder,
    bin: cfg.get<string>("bin", "rex"),
    enabled: cfg.get<boolean>("lint.enabled", true),
    projectOverride: cfg.get<ProjectOverride>("projectOverride", "auto"),
  };
}
