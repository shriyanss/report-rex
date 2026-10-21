import { ChildProcessWithoutNullStreams, spawn } from "child_process";
import * as readline from "readline";
import { RexConfig } from "./config";
import { connArgs, rexEnv } from "./cli";
import { backoffMs } from "./syncState";

// Mirrors cli/cmd/presence.go's presenceIn/presenceOut wire shapes exactly.
export interface PresenceUpdateOp {
  op: "update";
  file: string;
  state: "viewing" | "editing";
  dirty: boolean;
}
export interface PresenceReleaseOp {
  op: "release";
  file: string;
}
export type PresenceOp = PresenceUpdateOp | PresenceReleaseOp;

export interface PresenceEvent {
  type: "presence";
  username: string;
  session_id: string;
  file: string;
  state: string;
  dirty: boolean;
  action: "active" | "idle";
}
export interface StatusEvent {
  type: "status";
  connected?: boolean; // omitted (omitempty) when false
  error?: string;
}
// RemoteChangeEvent is a server file_updated broadcast: someone (possibly
// you, from another process) committed a change.
export interface RemoteChangeEvent {
  type: "remote_change";
  file: string;
  revision: number;
  modified_by: string;
  deleted?: boolean;
}
export interface ConflictCreatedEvent {
  type: "conflict_created";
  file: string;
  conflict_id: number;
  kind: string;
}
export type BridgeEvent = PresenceEvent | StatusEvent | RemoteChangeEvent | ConflictCreatedEvent;

// parseLine decodes one NDJSON line from `rex presence`. Returns null for a
// blank or malformed line -- presence is advisory, so a corrupt line is
// dropped rather than raised as an error (mirrors bridge.lua's parse_line).
export function parseLine(line: string): BridgeEvent | null {
  if (!line) {
    return null;
  }
  try {
    const decoded = JSON.parse(line);
    if (typeof decoded !== "object" || decoded === null || typeof decoded.type !== "string") {
      return null;
    }
    return decoded as BridgeEvent;
  } catch {
    return null;
  }
}

export interface PresenceBridge {
  send(op: PresenceOp): void;
  dispose(): void;
}

// startPresenceBridge spawns `<bin> presence ...` and bridges its NDJSON
// stdout to onEvent. This is the extension's always-on WebSocket: it runs
// whenever the workspace is configured, whether or not sync is started,
// and carries presence plus live remote changes and new conflicts. The
// network reconnect/backoff lives inside `rex presence`; this only restarts
// the process itself if it exits (bad password, rex missing, crash), with
// the same exponential backoff, reset once it connects.
export function startPresenceBridge(
  cfg: RexConfig,
  onEvent: (ev: BridgeEvent) => void,
  onLog: (text: string) => void
): PresenceBridge {
  let child: ChildProcessWithoutNullStreams | null = null;
  let disposed = false;
  let attempt = 0;
  let restartTimer: ReturnType<typeof setTimeout> | null = null;

  const spawnChild = () => {
    if (disposed) {
      return;
    }
    const proc = spawn(cfg.bin, ["presence", ...connArgs(cfg)], { env: rexEnv(cfg) });
    child = proc;
    readline.createInterface({ input: proc.stdout }).on("line", (line) => {
      const ev = parseLine(line);
      if (!ev) {
        return;
      }
      if (ev.type === "status" && ev.connected) {
        attempt = 0;
      }
      onEvent(ev);
    });
    let lastErr = "";
    readline.createInterface({ input: proc.stderr }).on("line", (line) => {
      lastErr = line;
      onLog(`rex presence: ${line}`);
    });
    proc.stdin.on("error", () => {
      // EPIPE after the child died; the exit handler restarts it.
    });
    let exited = false;
    const onGone = (why?: string) => {
      if (exited) {
        return;
      }
      exited = true;
      if (child === proc) {
        child = null;
      }
      if (disposed) {
        return;
      }
      onEvent({ type: "status", connected: false, error: why || lastErr || "rex presence exited" });
      restartTimer = setTimeout(spawnChild, backoffMs(attempt++));
    };
    proc.on("error", (e) => onGone(e.message));
    proc.on("exit", () => onGone());
  };
  spawnChild();

  return {
    send(op: PresenceOp) {
      if (!child) {
        return; // bridge not running: fail open, editing must never block on this
      }
      child.stdin.write(JSON.stringify(op) + "\n");
    },
    dispose() {
      disposed = true;
      if (restartTimer) {
        clearTimeout(restartTimer);
      }
      child?.kill();
      child = null;
    },
  };
}
