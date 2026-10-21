import * as path from "path";
import { ConflictSummary, FileEntry } from "./cli";
import { RemoteState } from "./presenceStore";
import { SyncState, PHASE_LABEL } from "./syncState";

// Pure view-model helpers behind the Report Rex views -- no `vscode`
// import, so they're testable under plain node:test.

export interface ConflictGroups {
  mine: ConflictSummary[];
  others: ConflictSummary[];
}

// splitConflicts separates the conflicts you can act on (your rejected
// changes) from teammates'. Newest first within each group.
export function splitConflicts(conflicts: ConflictSummary[], me: string): ConflictGroups {
  const sorted = [...conflicts].sort((a, b) => b.created_at.localeCompare(a.created_at));
  return {
    mine: sorted.filter((c) => c.local_username === me),
    others: sorted.filter((c) => c.local_username !== me),
  };
}

export const CONFLICT_KIND_LABEL: Record<string, string> = {
  divergent_edit: "both edited",
  initial_unknown_base: "both created",
  delete_edit: "edited vs deleted",
};

export interface RecentEntry {
  path: string;
  revision: number;
  modifiedBy: string;
  at: string; // RFC3339
  deleted: boolean;
}

export function recentFromStatus(files: FileEntry[]): RecentEntry[] {
  return files.map((f) => ({
    path: f.path,
    revision: f.revision,
    modifiedBy: f.modified_by,
    at: f.updated_at,
    deleted: f.deleted,
  }));
}

// mergeRecent puts entry at the top of list, replacing any older entry for
// the same path, and keeps at most limit entries.
export function mergeRecent(list: RecentEntry[], entry: RecentEntry, limit = 30): RecentEntry[] {
  const rest = list.filter((e) => e.path !== entry.path);
  return [entry, ...rest].slice(0, limit);
}

export function relativeTime(iso: string | undefined, nowMs = Date.now()): string {
  if (!iso) {
    return "never";
  }
  const t = Date.parse(iso);
  if (Number.isNaN(t)) {
    return iso;
  }
  const s = Math.max(0, Math.round((nowMs - t) / 1000));
  if (s < 10) {
    return "just now";
  }
  if (s < 60) {
    return `${s}s ago`;
  }
  if (s < 3600) {
    return `${Math.floor(s / 60)}m ago`;
  }
  if (s < 86400) {
    return `${Math.floor(s / 3600)}h ago`;
  }
  return `${Math.floor(s / 86400)}d ago`;
}

export interface TeamMember {
  username: string;
  files: { file: string; state: string; dirty: boolean }[];
}

// teamFromPresence groups live presence by teammate (excluding you), with
// the files each one has open -- editing first.
export function teamFromPresence(remote: RemoteState, me: string): TeamMember[] {
  const byUser = new Map<string, TeamMember>();
  for (const [file, sessions] of remote) {
    for (const e of sessions.values()) {
      if (e.username === me) {
        continue;
      }
      let m = byUser.get(e.username);
      if (!m) {
        m = { username: e.username, files: [] };
        byUser.set(e.username, m);
      }
      const existing = m.files.find((f) => f.file === file);
      if (!existing) {
        m.files.push({ file, state: e.state, dirty: e.dirty });
      } else if (e.state === "editing") {
        existing.state = "editing";
      }
    }
  }
  for (const m of byUser.values()) {
    m.files.sort((a, b) => (a.state === b.state ? a.file.localeCompare(b.file) : a.state === "editing" ? -1 : 1));
  }
  return [...byUser.values()].filter((m) => m.files.length > 0).sort((a, b) => a.username.localeCompare(b.username));
}

export interface HandoffInfo {
  path: string; // workspace-relative
  title: string;
  from: string;
  to: string;
  status: string;
}

// parseHandoff reads the frontmatter/title of a handoffs/<to>/<title>.md
// file (see cli/internal/handoff). Tolerant: missing fields are "".
export function parseHandoff(relPath: string, content: string): HandoffInfo {
  const fm = /^---\r?\n([\s\S]*?)\r?\n---/.exec(content)?.[1] ?? "";
  const field = (k: string) => new RegExp(`^${k}:\\s*"?([^"\\r\\n]*)"?\\s*$`, "m").exec(fm)?.[1].trim() ?? "";
  const heading = /^#\s+(.+)$/m.exec(content.slice(fm ? fm.length + 8 : 0))?.[1].trim();
  return {
    path: relPath,
    title: heading || path.posix.basename(relPath, ".md"),
    from: field("from"),
    to: field("to") || relPath.split("/")[1] || "",
    status: field("status") || "open",
  };
}

// relativeTo returns fsPath relative to dir with forward slashes, or null
// when it's outside dir -- presence and conflict paths are relative to the
// synced directory (report-rex.dir), which needn't be the workspace root.
export function relativeTo(dir: string, fsPath: string): string | null {
  if (!dir) {
    return null;
  }
  const rel = path.relative(dir, fsPath);
  if (!rel || rel.startsWith("..") || path.isAbsolute(rel)) {
    return null;
  }
  return rel.split(path.sep).join("/");
}

// hasConflictMarkers matches cli/internal/syncclient/merge.go's markers.
export function hasConflictMarkers(text: string): boolean {
  return text.includes("<<<<<<< LOCAL") && text.includes("=======") && text.includes(">>>>>>> REMOTE");
}

const PHASE_ICON: Record<string, string> = {
  stopped: "$(circle-slash)",
  starting: "$(sync~spin)",
  connected: "$(cloud)",
  offline: "$(debug-disconnect)",
  reconnecting: "$(sync~spin)",
  error: "$(error)",
};

// statusBarText renders the one Report Rex status bar item: sync state,
// your open conflicts, and who else is on the active file.
export function statusBarText(sync: SyncState, myConflicts: number, presence: string): string {
  let text = `${PHASE_ICON[sync.phase]} Rex`;
  if (sync.phase === "stopped") {
    text += " (sync off)";
  } else if (sync.phase === "offline") {
    text += " offline";
  }
  if (myConflicts > 0) {
    text += ` $(warning) ${myConflicts}`;
  }
  if (presence) {
    text += ` · ${presence}`;
  }
  return text;
}

export function statusBarTooltip(sync: SyncState, myConflicts: number, server: string): string {
  const lines = [`Report Rex sync: ${PHASE_LABEL[sync.phase]}${server ? ` — ${server}` : ""}`];
  if (sync.lastSyncAt) {
    lines.push(`Last change synced ${relativeTime(sync.lastSyncAt)}`);
  }
  if (sync.lastError) {
    lines.push(`Last error: ${sync.lastError}`);
  }
  if (myConflicts > 0) {
    lines.push(`${myConflicts} open conflict${myConflicts === 1 ? "" : "s"} of yours — open the Report Rex view to resolve`);
  }
  return lines.join("\n");
}
