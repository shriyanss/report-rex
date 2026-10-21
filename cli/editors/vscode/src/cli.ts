import { spawn } from "child_process";
import { RexConfig } from "./config";
import { tlsArgs } from "./tlsArgs";

// Everything the extension asks of rex-svr goes through the `rex` binary,
// which already owns TLS pinning, login, and the wire protocol -- this
// module only builds argv/env and runs it. No `vscode` import (RexConfig is
// type-only), so it stays testable under plain node:test.

// MIN_REX_VERSION is the first rex release with `sync --json`, `status`,
// and `conflicts --json/--side`.
export const MIN_REX_VERSION = "0.7.0";

export type ConnConfig = Pick<RexConfig, "host" | "port" | "username" | "insecure" | "caCert" | "password" | "bin">;

export function connArgs(cfg: ConnConfig): string[] {
  return ["--host", cfg.host, "--port", String(cfg.port), "--username", cfg.username, ...tlsArgs(cfg)];
}

// rexEnv passes the password via $REX_PASSWORD rather than --password, so
// it never shows up in the process list.
export function rexEnv(cfg: Pick<RexConfig, "password">): NodeJS.ProcessEnv {
  return { ...process.env, ...(cfg.password ? { REX_PASSWORD: cfg.password } : {}) };
}

// Mirrors syncproto.RevertPreview (`rex revert --json`). content is base64,
// as Go encodes []byte.
export interface RevertOp {
  path: string;
  action: "write" | "delete";
  from_revision: number;
  target_revision?: number;
  content?: string;
}
export interface RevertPreview {
  path: string;
  revision: number;
  before?: boolean;
  commit?: string;
  ops: RevertOp[];
  plan: string;
  git_log: string[];
}

export function revertArgs(path: string, revision: number, before: boolean, plan?: string): string[] {
  return [
    "revert",
    "--json",
    "--path",
    path,
    "--revision",
    String(revision),
    ...(before ? ["--before"] : []),
    ...(plan ? ["--plan", plan] : []),
  ];
}

export interface RexResult {
  code: number | null;
  stdout: string;
  stderr: string;
  error?: string; // spawn failure, e.g. the binary isn't on $PATH
}

export function runRex(
  bin: string,
  args: string[],
  opts: { env?: NodeJS.ProcessEnv; cwd?: string; timeoutMs?: number } = {}
): Promise<RexResult> {
  return new Promise((resolve) => {
    let stdout = "";
    let stderr = "";
    let proc;
    try {
      proc = spawn(bin, args, { env: opts.env, cwd: opts.cwd, stdio: ["ignore", "pipe", "pipe"] });
    } catch (e) {
      resolve({ code: null, stdout, stderr, error: String(e) });
      return;
    }
    const timer = opts.timeoutMs ? setTimeout(() => proc.kill(), opts.timeoutMs) : undefined;
    proc.stdout.on("data", (d) => (stdout += d));
    proc.stderr.on("data", (d) => (stderr += d));
    proc.on("error", (e) => {
      clearTimeout(timer);
      resolve({ code: null, stdout, stderr, error: e.message });
    });
    proc.on("close", (code) => {
      clearTimeout(timer);
      resolve({ code, stdout, stderr });
    });
  });
}

// Mirrors cli/cmd/status.go's statusReport.
export interface ConflictSummary {
  id: number;
  path: string;
  kind: string;
  remote_revision: number;
  local_username: string;
  created_at: string;
}
export interface FileEntry {
  path: string;
  revision: number;
  deleted: boolean;
  modified_by: string;
  updated_at: string;
}
export interface StatusReport {
  server: string;
  user: string;
  reachable: boolean;
  latency_ms: number;
  health?: { status: string; git_ok: boolean; db_ok: boolean; pending_materializations: number; version?: string };
  authenticated: boolean;
  files: number;
  recent: FileEntry[];
  conflicts: ConflictSummary[];
  error?: string;
  hint?: "config" | "tls" | "unreachable" | "auth" | "server";
}

// parseStatus decodes `rex status --json`. A spawn failure or non-JSON
// output (e.g. an old rex without `status`) becomes a synthetic failed
// report, so callers only ever deal with one shape.
export function parseStatus(res: RexResult): StatusReport {
  try {
    const r = JSON.parse(res.stdout) as StatusReport;
    if (typeof r === "object" && r && typeof r.reachable === "boolean") {
      return r;
    }
  } catch {
    // fall through
  }
  const error = res.error
    ? `could not run rex: ${res.error}`
    : res.stderr.trim() || `rex status exited with code ${res.code}`;
  return {
    server: "",
    user: "",
    reachable: false,
    latency_ms: 0,
    authenticated: false,
    files: 0,
    recent: [],
    conflicts: [],
    error,
    hint: res.error ? "config" : undefined,
  };
}

// HINT_ADVICE says what to do about each failure layer `rex status` reports.
export const HINT_ADVICE: Record<string, string> = {
  config: "Check the host, username, password, and report-rex.bin settings.",
  tls: "Trust the server's certificate: set its CA cert (printed by rex-svr on startup), or enable insecure mode on a trusted network.",
  unreachable: "The server didn't answer: check the host/port, your network or VPN, and that rex-svr is running.",
  auth: "The server rejected the password.",
  server: "The server answered but failed the request; check the rex-svr logs.",
};

// compareVersions compares dotted numeric versions ("v0.7.0", "0.6.2");
// pre-release/build suffixes are ignored.
export function compareVersions(a: string, b: string): number {
  const parts = (v: string) =>
    v
      .replace(/^v/, "")
      .split(/[-+]/)[0]
      .split(".")
      .map((n) => parseInt(n, 10) || 0);
  const pa = parts(a);
  const pb = parts(b);
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d !== 0) {
      return d;
    }
  }
  return 0;
}

// rexVersionOk accepts local/dev builds (no numeric version) -- a
// developer running from source is by definition on the latest code.
export function rexVersionOk(version: string, min = MIN_REX_VERSION): boolean {
  const v = version.trim();
  if (!/^v?\d+\.\d+/.test(v)) {
    return true;
  }
  return compareVersions(v, min) >= 0;
}

// adminVerifyArgs builds `rex admin verify`; the admin password goes in
// $REX_ADMIN_PASSWORD.
export function adminVerifyArgs(): string[] {
  return ["admin", "verify"];
}

// deleteArgs builds `rex delete` for workspace-relative paths; the admin
// password goes in $REX_ADMIN_PASSWORD.
export function deleteArgs(paths: string[]): string[] {
  return ["delete", "--json", ...paths];
}

// Mirrors syncproto.BulkDeleteResponse (`rex delete --json`).
export interface BulkDeleteResult {
  deleted: { path: string; revision: number }[];
  skipped: string[];
  commit?: string;
}

export interface VulnTemplate {
  name: string;
  title: string;
}

// parseTemplateList reads `rex report section vuln --list-templates`
// (one `name<TAB>title` line per vuln-templates/ stub).
export function parseTemplateList(stdout: string): VulnTemplate[] {
  return stdout
    .split("\n")
    .filter((l) => l.trim())
    .map((l) => {
      const [name, ...rest] = l.split("\t");
      return { name: name.trim(), title: rest.join("\t").trim() || name.trim() };
    });
}

// Mirrors syncproto.BlameResponse (`rex blame --json`).
export interface BlameLine {
  line: number;
  commit?: string;
  user: string;
  time: string;
  summary: string;
  text: string;
}
export interface BlameResponse {
  path: string;
  lines: BlameLine[];
}

export function blameArgs(path: string): string[] {
  return ["blame", "--json", path];
}

// timeAgo renders an RFC 3339 time as "5 minutes ago".
export function timeAgo(time: string, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - Date.parse(time)) / 1000));
  if (!Number.isFinite(s)) {
    return "";
  }
  const units: [string, number][] = [["year", 31536000], ["month", 2592000], ["day", 86400], ["hour", 3600], ["minute", 60]];
  for (const [name, secs] of units) {
    if (s >= secs) {
      const n = Math.floor(s / secs);
      return `${n} ${name}${n > 1 ? "s" : ""} ago`;
    }
  }
  return "just now";
}

// blameAnnotation is the GitLens-style end-of-line text for one line.
export function blameAnnotation(l: BlameLine, now = Date.now()): string {
  return `${l.user}, ${timeAgo(l.time, now)} • ${l.summary}`;
}

// formatBlame renders a whole file's blame for the Rex Blame tab.
export function formatBlame(b: BlameResponse): string {
  const width = Math.max(4, ...b.lines.map((l) => l.user.length));
  return b.lines
    .map((l) => `${(l.commit ?? "").slice(0, 8).padEnd(8)} ${l.user.padEnd(width)} ${l.time.slice(0, 10).padEnd(10)} ${String(l.line).padStart(4)}| ${l.text}`)
    .join("\n");
}
