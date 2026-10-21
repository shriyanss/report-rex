import { ChildProcessWithoutNullStreams, spawn } from "child_process";
import * as readline from "readline";
import { RexConfig } from "./config";
import { connArgs, rexEnv } from "./cli";
import { SyncLine, backoffMs, parseSyncLine } from "./syncState";

export interface SyncBridgeHandlers {
  onLine(line: SyncLine): void;
  onLog(text: string): void;
  // onCrash fires when the process exits without being asked to; the
  // bridge has already scheduled a restart restartInMs from now.
  onCrash(code: number | null, stderrTail: string, restartInMs: number, attempt: number): void;
}

// SyncBridge keeps one `rex sync --json` process alive while started. It
// always drains stdout and stderr -- an unread pipe fills up and blocks the
// child mid-write, which is how the old bridge (reading only in verbose
// mode) could silently freeze sync.
export class SyncBridge {
  private child: ChildProcessWithoutNullStreams | null = null;
  private wanted = false;
  private attempt = 0;
  private restartTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(private cfg: RexConfig, private readonly h: SyncBridgeHandlers) {}

  get running(): boolean {
    return this.wanted;
  }

  start(): void {
    if (this.wanted) {
      return;
    }
    this.wanted = true;
    this.attempt = 0;
    this.spawnChild();
  }

  stop(): void {
    this.wanted = false;
    if (this.restartTimer) {
      clearTimeout(this.restartTimer);
      this.restartTimer = null;
    }
    const proc = this.child;
    this.child = null;
    if (proc) {
      // Closing stdin asks `rex sync --json` to stop cleanly; SIGTERM
      // backs that up.
      proc.stdin.end();
      proc.kill();
    }
  }

  // restart applies a new config: stop now, start fresh if it was running.
  restart(cfg: RexConfig): void {
    const was = this.wanted;
    this.stop();
    this.cfg = cfg;
    if (was) {
      this.start();
    }
  }

  syncNow(): void {
    this.child?.stdin.write(JSON.stringify({ op: "reconcile" }) + "\n");
  }

  private spawnChild(): void {
    const proc = spawn(this.cfg.bin, ["sync", "--json", "--dir", this.cfg.dir, ...connArgs(this.cfg)], {
      env: rexEnv(this.cfg),
      cwd: this.cfg.dir,
    });
    this.child = proc;
    let stderrTail = "";
    readline.createInterface({ input: proc.stdout }).on("line", (raw) => {
      if (this.child !== proc) {
        return; // a stopped/replaced process's last words (e.g. "stopped") aren't current state
      }
      const line = parseSyncLine(raw);
      if (!line) {
        this.h.onLog(`rex sync: ${raw}`);
        return;
      }
      if (line.type === "ready") {
        this.attempt = 0; // it came up: the next crash starts the backoff over
      }
      this.h.onLine(line);
    });
    readline.createInterface({ input: proc.stderr }).on("line", (raw) => {
      stderrTail = (stderrTail + "\n" + raw).slice(-2000);
      this.h.onLog(`rex sync: ${raw}`);
    });
    proc.stdin.on("error", () => {
      // EPIPE after the child died; the exit handler deals with it.
    });
    let exited = false;
    const onGone = (code: number | null, why?: string) => {
      if (exited) {
        return;
      }
      exited = true;
      if (why) {
        stderrTail = (stderrTail + "\n" + why).slice(-2000);
      }
      if (this.child !== proc) {
        return; // stopped or replaced on purpose: not a crash
      }
      this.child = null;
      if (!this.wanted) {
        return;
      }
      const delay = backoffMs(this.attempt++);
      this.restartTimer = setTimeout(() => {
        this.restartTimer = null;
        if (this.wanted) {
          this.spawnChild();
        }
      }, delay);
      this.h.onCrash(code, stderrTail.trim(), delay, this.attempt);
    };
    proc.on("error", (e) => onGone(null, e.message));
    proc.on("exit", (code) => onGone(code));
  }
}
