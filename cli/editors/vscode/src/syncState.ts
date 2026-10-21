// Pure state for the background `rex sync --json` process -- no `vscode`
// import, so it's testable under plain node:test.

// Mirrors cli/cmd/sync.go's syncLine.
export interface SyncLine {
  type: "ready" | "status" | "event" | "error";
  time: string;
  state?: "connected" | "offline" | "reconnecting" | "stopped";
  kind?: "pushed" | "pulled" | "deleted" | "conflict" | "merged" | "skipped" | "updated";
  path?: string;
  detail?: string;
  conflict_id?: number;
  conflict_kind?: string;
  scope?: string;
  message?: string;
  server?: string;
  user?: string;
  dir?: string;
}

export function parseSyncLine(line: string): SyncLine | null {
  if (!line) {
    return null;
  }
  try {
    const l = JSON.parse(line);
    return typeof l === "object" && l !== null && typeof l.type === "string" ? (l as SyncLine) : null;
  } catch {
    return null;
  }
}

export type SyncPhase = "stopped" | "starting" | "connected" | "offline" | "reconnecting" | "error";

export interface SyncState {
  phase: SyncPhase;
  lastError?: string;
  lastSyncAt?: string; // time of the last pushed/pulled/deleted/merged event
  pushed: number;
  pulled: number;
  merged: number;
  conflicts: number;
}

export function initialSyncState(): SyncState {
  return { phase: "stopped", pushed: 0, pulled: 0, merged: 0, conflicts: 0 };
}

// nextSyncState folds one `rex sync --json` line into the state. It never
// mutates its input.
export function nextSyncState(s: SyncState, l: SyncLine): SyncState {
  switch (l.type) {
    case "ready":
      // Logged in and running the first pass; the WebSocket may not be up
      // yet, but HTTP sync already works.
      return { ...s, phase: "connected", lastError: undefined };
    case "status":
      if (l.state === "stopped") {
        return { ...s, phase: "stopped" };
      }
      if (l.state === "offline") {
        return { ...s, phase: "offline", lastError: l.message };
      }
      if (l.state === "connected" || l.state === "reconnecting") {
        return { ...s, phase: l.state, lastError: l.state === "connected" ? undefined : s.lastError };
      }
      return s;
    case "event": {
      const n = { ...s };
      if (l.kind === "pushed" || l.kind === "deleted") {
        n.pushed++;
      } else if (l.kind === "pulled") {
        n.pulled++;
      } else if (l.kind === "merged") {
        n.merged++;
      } else if (l.kind === "conflict") {
        n.conflicts++;
      }
      if (l.kind !== "conflict" && l.kind !== "skipped") {
        n.lastSyncAt = l.time;
      }
      return n;
    }
    case "error":
      return { ...s, lastError: l.message };
    default:
      return s;
  }
}

// backoffMs is the restart delay after the attempt-th consecutive crash:
// 1s, 2s, 4s, ... capped at 30s -- the same curve rex uses for its own
// WebSocket reconnects.
export function backoffMs(attempt: number): number {
  return Math.min(30_000, 1000 * 2 ** Math.max(0, attempt));
}

export const PHASE_LABEL: Record<SyncPhase, string> = {
  stopped: "Stopped",
  starting: "Starting…",
  connected: "Connected",
  offline: "Offline (working locally)",
  reconnecting: "Reconnecting…",
  error: "Error",
};
