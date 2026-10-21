import * as fs from "fs";
import * as path from "path";
import * as vscode from "vscode";
import { RexConfig, resolveConfig } from "./config";
import {
  BlameResponse,
  BulkDeleteResult,
  adminVerifyArgs,
  ConflictSummary,
  HINT_ADVICE,
  RevertPreview,
  StatusReport,
  blameArgs,
  connArgs,
  deleteArgs,
  formatBlame,
  parseStatus,
  revertArgs,
  rexEnv,
  runRex,
} from "./cli";
import { BridgeEvent, PresenceBridge, PresenceOp, startPresenceBridge } from "./presenceBridge";
import { RemoteState, applyPresenceEvent, createRemoteState } from "./presenceStore";
import { SyncBridge } from "./syncBridge";
import { SyncLine, SyncState, initialSyncState, nextSyncState } from "./syncState";
import { RecentEntry, mergeRecent, recentFromStatus, splitConflicts } from "./model";

export type Resolution = "local" | "remote" | "merge" | "keep-both";

const RESOLUTION_LABEL: Record<Resolution, string> = {
  local: "kept your version",
  remote: "took the server's version",
  merge: "merged",
  "keep-both": "kept both (yours saved as a .conflict copy)",
};

const REFRESH_EVERY_MS = 60_000;

// RexController owns everything stateful: the always-on presence bridge,
// the optional sync bridge, and the server snapshot (health, conflicts,
// recent changes) the views render. Views and commands only read from it
// and call its methods.
export class RexController implements vscode.Disposable {
  readonly remote: RemoteState = createRemoteState();
  sync: SyncState = initialSyncState();
  health?: StatusReport;
  healthCheckedAt?: string;
  conflicts: ConflictSummary[] = [];
  recent: RecentEntry[] = [];
  presenceConnected?: boolean;
  cfg: RexConfig | null = null;
  // Files with a `rex resolve --merge` in progress (markers written), by
  // workspace-relative path.
  readonly pendingMerges = new Map<string, number>();

  private presence: PresenceBridge | null = null;
  private syncBridge: SyncBridge | null = null;
  private readonly changed = new vscode.EventEmitter<void>();
  readonly onDidChange = this.changed.event;
  private refreshTimer?: ReturnType<typeof setTimeout>;
  private readonly poll: ReturnType<typeof setInterval>;
  private readonly notifiedConflicts = new Set<number>();
  private crashNotified = false;
  private massDeleteNotified = false;
  private refreshing: Promise<void> | null = null;
  // The revert being previewed, backing the rex-revert: diff documents.
  private revertPreview?: RevertPreview;
  // The rex-svr admin password, once the server has accepted it. Session
  // only; cleared on reconnect, when Enable Admin is turned off, or when the
  // server rejects it.
  private adminPassword?: string;

  constructor(private readonly context: vscode.ExtensionContext, readonly log: vscode.OutputChannel) {
    this.poll = setInterval(() => void this.refresh(), REFRESH_EVERY_MS);
  }

  get me(): string {
    return this.cfg?.username ?? "";
  }

  get syncRunning(): boolean {
    return !!this.syncBridge?.running;
  }

  myConflicts(): ConflictSummary[] {
    return splitConflicts(this.conflicts, this.me).mine;
  }

  // abs maps a workspace-relative (sync-dir-relative) path to a file URI.
  abs(rel: string): vscode.Uri | undefined {
    return this.cfg ? vscode.Uri.file(path.join(this.cfg.dir, ...rel.split("/"))) : undefined;
  }

  fire(): void {
    void vscode.commands.executeCommand("setContext", "reportRex.configured", !!this.cfg);
    void vscode.commands.executeCommand("setContext", "reportRex.syncRunning", this.syncRunning);
    void vscode.commands.executeCommand("setContext", "reportRex.hasPendingMerge", this.pendingMerges.size > 0);
    void vscode.commands.executeCommand("setContext", "reportRex.admin", adminEnabled());
    this.changed.fire();
  }

  sendPresence(op: PresenceOp): void {
    this.presence?.send(op);
  }

  // reload re-reads settings/password and (re)starts the bridges to match.
  // Sync resumes if this workspace had it enabled, without the interactive
  // connection test -- the bridge's backoff copes with a server that's down.
  async reload(): Promise<void> {
    this.presence?.dispose();
    this.presence = null;
    this.remote.clear();
    this.presenceConnected = undefined;
    this.adminPassword = undefined;
    this.cfg = await resolveConfig(this.context);
    const enabled = vscode.workspace.getConfiguration("report-rex").get<boolean>("sync.enabled", false);
    if (!this.cfg || !this.cfg.password) {
      this.stopSync(false);
      this.fire();
      return;
    }
    this.presence = startPresenceBridge(this.cfg, (ev) => this.onPresenceEvent(ev), (t) => this.log.appendLine(t));
    if (this.syncBridge) {
      this.syncBridge.restart(this.cfg);
    }
    if (enabled && !this.syncRunning) {
      this.startSyncBridge();
    } else if (!enabled && this.syncRunning) {
      this.stopSync(false);
    }
    this.fire();
    void this.refresh();
  }

  // onAdminSettingChanged follows report-rex.admin.enabled: turning it on
  // asks for the admin password right away, turning it off forgets it.
  async onAdminSettingChanged(): Promise<void> {
    this.adminPassword = undefined;
    this.fire();
    if (adminEnabled()) {
      await this.admin();
    }
  }

  // admin returns the rex-svr admin password, prompting for it and checking
  // it against the server the first time; undefined if Enable Admin is off,
  // the prompt was cancelled, or the server refused it.
  async admin(): Promise<string | undefined> {
    const cfg = this.cfg;
    if (!cfg || !adminEnabled()) {
      return undefined;
    }
    if (this.adminPassword) {
      return this.adminPassword;
    }
    const pw = await vscode.window.showInputBox({
      prompt: "rex-svr admin password (unlocks admin features for this session)",
      password: true,
      ignoreFocusOut: true,
    });
    if (!pw) {
      return undefined;
    }
    const res = await runRex(cfg.bin, [...adminVerifyArgs(), ...connArgs(cfg)], {
      env: { ...rexEnv(cfg), REX_ADMIN_PASSWORD: pw },
      timeoutMs: 20_000,
    });
    if (res.code !== 0) {
      void vscode.window.showErrorMessage(`Report Rex: admin unlock failed: ${res.stderr.trim() || res.error}`);
      return undefined;
    }
    this.adminPassword = pw;
    void vscode.window.showInformationMessage("Report Rex: admin features unlocked.");
    return pw;
  }

  // adminFailed reports a failed admin action, forgetting the cached admin
  // password if that's what the server rejected.
  private adminFailed(what: string, stderr: string): void {
    if (stderr.includes("wrong admin password")) {
      this.adminPassword = undefined;
    }
    void vscode.window.showErrorMessage(`Report Rex: ${what} failed: ${stderr}`);
  }

  // onSyncSettingChanged follows report-rex.sync.enabled when something
  // other than Start/Stop (Settings UI, settings.json) flips it.
  onSyncSettingChanged(): void {
    const enabled = vscode.workspace.getConfiguration("report-rex").get<boolean>("sync.enabled", false);
    if (enabled && !this.syncRunning && this.cfg?.password) {
      this.startSyncBridge();
    } else if (!enabled && this.syncRunning) {
      this.stopSync(false);
    }
  }

  // startSync is the Start button: make sure the workspace is configured,
  // test the connection, and only then turn sync on -- a failure explains
  // what to fix instead of silently leaving a dead process behind.
  async startSync(): Promise<boolean> {
    this.cfg = await resolveConfig(this.context);
    if (!this.cfg) {
      const pick = await vscode.window.showWarningMessage(
        "Report Rex: set the server host and your username before starting sync.",
        "Set Host",
        "Set Username"
      );
      if (pick) {
        await vscode.commands.executeCommand(pick === "Set Host" ? "report-rex.setHost" : "report-rex.setUsername");
      }
      return false;
    }
    if (!this.cfg.password) {
      await vscode.commands.executeCommand("report-rex.setPassword");
      this.cfg = await resolveConfig(this.context);
      if (!this.cfg?.password) {
        return false;
      }
    }
    const rep = await this.testConnection(false);
    if (!rep || rep.error) {
      return false;
    }
    await vscode.workspace.getConfiguration("report-rex").update("sync.enabled", true, vscode.ConfigurationTarget.Workspace);
    if (!this.presence) {
      await this.reload();
    }
    this.startSyncBridge();
    vscode.window.setStatusBarMessage(`$(sync) Report Rex: syncing ${this.cfg.dir}`, 4000);
    return true;
  }

  private startSyncBridge(): void {
    if (!this.cfg) {
      return;
    }
    if (!this.syncBridge) {
      this.syncBridge = new SyncBridge(this.cfg, {
        onLine: (l) => this.onSyncLine(l),
        onLog: (t) => this.log.appendLine(t),
        onCrash: (code, tail, delay, attempt) => this.onSyncCrash(code, tail, delay, attempt),
      });
    }
    this.sync = { ...this.sync, phase: "starting", lastError: undefined };
    this.syncBridge.start();
    this.fire();
  }

  stopSync(persist = true): void {
    this.syncBridge?.stop();
    this.sync = { ...this.sync, phase: "stopped" };
    if (persist) {
      void vscode.workspace
        .getConfiguration("report-rex")
        .update("sync.enabled", false, vscode.ConfigurationTarget.Workspace);
    }
    this.fire();
  }

  syncNow(): void {
    this.syncBridge?.syncNow();
    this.scheduleRefresh(2000);
  }

  private onSyncLine(l: SyncLine): void {
    const prev = this.sync.phase;
    this.sync = nextSyncState(this.sync, l);
    if (l.type === "ready") {
      this.crashNotified = false;
      this.massDeleteNotified = false;
    }
    if (l.type === "event") {
      const detail = l.detail ? ` (${l.detail})` : "";
      this.log.appendLine(`[${l.time}] ${l.kind} ${l.path}${detail}`);
      // A path-less skip is sync refusing a mass delete (issue #46); it
      // repeats every pass, so warn once per sync run.
      if (l.kind === "skipped" && !l.path && !this.massDeleteNotified) {
        this.massDeleteNotified = true;
        void vscode.window.showWarningMessage(`Report Rex: ${l.detail}`);
      }
      if (l.kind === "conflict" && l.conflict_id) {
        this.notifyConflict(l.path ?? "", l.conflict_id);
      }
      if (l.kind === "conflict" || l.kind === "merged") {
        this.scheduleRefresh();
      }
    } else if (l.type === "error") {
      this.log.appendLine(`[${l.time}] ${l.scope} error: ${l.message}`);
    } else if (l.type === "status") {
      if (l.state === "offline" && prev === "connected") {
        void vscode.window.showWarningMessage(
          `Report Rex: lost the connection to ${this.cfg?.host} (${l.message ?? "unknown error"}). ` +
            "Keep working -- everything syncs when it reconnects."
        );
      } else if (l.state === "connected" && (prev === "offline" || prev === "reconnecting")) {
        vscode.window.setStatusBarMessage("$(cloud) Report Rex: reconnected, syncing…", 4000);
        this.scheduleRefresh();
      }
    }
    this.fire();
  }

  private onSyncCrash(code: number | null, tail: string, delay: number, attempt: number): void {
    const last = tail.split("\n").filter(Boolean).pop() ?? `exited with code ${code}`;
    this.sync = { ...this.sync, phase: "error", lastError: last };
    this.log.appendLine(`rex sync exited (${code}); restarting in ${delay / 1000}s: ${last}`);
    this.fire();
    if (attempt >= 3 && !this.crashNotified) {
      this.crashNotified = true;
      void vscode.window
        .showErrorMessage(`Report Rex: sync keeps failing: ${last}`, "Test Connection", "Show Log", "Stop Sync")
        .then((pick) => {
          if (pick === "Test Connection") {
            void this.testConnection(true);
          } else if (pick === "Show Log") {
            this.log.show();
          } else if (pick === "Stop Sync") {
            this.stopSync();
          }
        });
    }
  }

  private onPresenceEvent(ev: BridgeEvent): void {
    switch (ev.type) {
      case "presence":
        applyPresenceEvent(this.remote, ev);
        break;
      case "status":
        if (!ev.connected && ev.error) {
          this.log.appendLine(`rex presence: ${ev.error}`);
        }
        if (ev.connected && !this.presenceConnected) {
          this.scheduleRefresh(500);
        }
        this.presenceConnected = !!ev.connected;
        break;
      case "remote_change":
        this.recent = mergeRecent(this.recent, {
          path: ev.file,
          revision: ev.revision,
          modifiedBy: ev.modified_by,
          at: new Date().toISOString(),
          deleted: !!ev.deleted,
        });
        break;
      case "conflict_created":
        this.scheduleRefresh();
        break;
    }
    this.fire();
  }

  private notifyConflict(file: string, id: number): void {
    if (this.notifiedConflicts.has(id)) {
      return; // one open conflict per (path, you) -- don't re-toast every save
    }
    this.notifiedConflicts.add(id);
    void vscode.window
      .showWarningMessage(
        `Report Rex: sync conflict on ${file} -- you and a teammate changed the same lines. ` +
          "Your version is kept locally until you choose.",
        "Compare",
        "Resolve…",
        "Show Conflicts"
      )
      .then(async (pick) => {
        const c = { id, path: file, kind: "divergent_edit", remote_revision: 0, local_username: this.me, created_at: "" };
        if (pick === "Compare") {
          await this.compare(this.conflicts.find((x) => x.id === id) ?? c);
        } else if (pick === "Resolve…") {
          await vscode.commands.executeCommand("report-rex.conflict.resolve", this.conflicts.find((x) => x.id === id) ?? c);
        } else if (pick === "Show Conflicts") {
          await vscode.commands.executeCommand("reportRex.conflicts.focus");
        }
      });
  }

  scheduleRefresh(ms = 1500): void {
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer);
    }
    this.refreshTimer = setTimeout(() => void this.refresh(), ms);
  }

  // status runs `rex status --json` with the current config.
  private async status(): Promise<StatusReport | undefined> {
    if (!this.cfg) {
      return undefined;
    }
    const res = await runRex(this.cfg.bin, ["status", "--json", "--recent", "30", ...connArgs(this.cfg)], {
      env: rexEnv(this.cfg),
      timeoutMs: 20_000,
    });
    return parseStatus(res);
  }

  private apply(rep: StatusReport): void {
    this.health = rep;
    this.healthCheckedAt = new Date().toISOString();
    if (!rep.error) {
      this.conflicts = rep.conflicts;
      this.recent = recentFromStatus(rep.recent);
      const open = new Set(rep.conflicts.map((c) => c.id));
      for (const [file, id] of this.pendingMerges) {
        if (!open.has(id)) {
          this.pendingMerges.delete(file);
        }
      }
    }
    this.fire();
  }

  // refresh quietly re-pulls the server snapshot. Coalesces overlapping
  // calls.
  refresh(): Promise<void> {
    if (!this.cfg?.password) {
      return Promise.resolve();
    }
    if (!this.refreshing) {
      this.refreshing = (async () => {
        const rep = await this.status();
        if (rep) {
          this.apply(rep);
        }
      })().finally(() => (this.refreshing = null));
    }
    return this.refreshing;
  }

  // testConnection is the Test Connection button; with show=false it only
  // reports failures (used by Start).
  async testConnection(show: boolean): Promise<StatusReport | undefined> {
    this.cfg = await resolveConfig(this.context);
    if (!this.cfg) {
      void vscode.window.showWarningMessage("Report Rex: set the server host and your username first.");
      return undefined;
    }
    const rep = await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: `Report Rex: testing ${this.cfg.host}:${this.cfg.port}…` },
      () => this.status()
    );
    if (!rep) {
      return undefined;
    }
    this.apply(rep);
    if (rep.error) {
      this.log.appendLine(`test connection failed (${rep.hint}): ${rep.error}`);
      const actions: Record<string, string[]> = {
        tls: ["Configure TLS"],
        auth: ["Set Password"],
        unreachable: ["Set Host", "Set Port"],
        config: ["Set Host", "Set Username", "Set Password"],
      };
      const pick = await vscode.window.showErrorMessage(
        `Report Rex: can't connect (${rep.hint ?? "error"}): ${rep.error}. ${HINT_ADVICE[rep.hint ?? ""] ?? ""}`,
        ...(actions[rep.hint ?? ""] ?? []),
        "Show Log"
      );
      const cmd: Record<string, string> = {
        "Configure TLS": "report-rex.configureTls",
        "Set Password": "report-rex.setPassword",
        "Set Host": "report-rex.setHost",
        "Set Port": "report-rex.setPort",
        "Set Username": "report-rex.setUsername",
      };
      if (pick === "Show Log") {
        this.log.show();
      } else if (pick) {
        await vscode.commands.executeCommand(cmd[pick]);
      }
    } else if (show) {
      const mine = splitConflicts(rep.conflicts, this.me).mine.length;
      void vscode.window.showInformationMessage(
        `Report Rex: connected to ${rep.server} as ${rep.user}` +
          (rep.health?.version ? ` — rex-svr ${rep.health.version}` : "") +
          `, ${rep.files} files, ${rep.conflicts.length} open conflicts (${mine} yours), ${rep.latency_ms} ms.`
      );
    }
    return rep;
  }

  // resolve settles one conflict via `rex resolve`.
  async resolve(c: ConflictSummary, how: Resolution): Promise<boolean> {
    if (!this.cfg) {
      return false;
    }
    const res = await runRex(
      this.cfg.bin,
      ["resolve", c.path, "--id", String(c.id), `--${how}`, "--dir", this.cfg.dir, ...connArgs(this.cfg)],
      { env: rexEnv(this.cfg), cwd: this.cfg.dir, timeoutMs: 60_000 }
    );
    const out = (res.stdout + res.stderr).trim();
    this.log.appendLine(`rex resolve ${c.path} --id ${c.id} --${how}: ${out}`);
    if (res.code !== 0) {
      void vscode.window.showErrorMessage(`Report Rex: couldn't resolve ${c.path}: ${out || res.error}`);
      return false;
    }
    if (how === "merge" && out.includes("Automatic merge failed")) {
      this.pendingMerges.set(c.path, c.id);
      const uri = this.abs(c.path);
      if (uri) {
        await vscode.window.showTextDocument(uri);
      }
      void vscode.window.showInformationMessage(
        `Report Rex: ${c.path} has overlapping changes. Pick a side for each <<<<<<< LOCAL / >>>>>>> REMOTE block ` +
          "(the Accept Current/Incoming buttons above each block), save, and Report Rex submits the result."
      );
      this.fire();
      return true;
    }
    this.pendingMerges.delete(c.path);
    this.notifiedConflicts.delete(c.id);
    vscode.window.setStatusBarMessage(`$(check) Report Rex: ${c.path} — ${RESOLUTION_LABEL[how]}`, 5000);
    this.syncNow();
    await this.refresh();
    return true;
  }

  // compare opens a diff of the server's copy against yours.
  async compare(c: ConflictSummary): Promise<void> {
    const local = this.abs(c.path);
    if (!local) {
      return;
    }
    const q = (side: string) =>
      vscode.Uri.from({ scheme: CONFLICT_SCHEME, path: "/" + c.path, query: `id=${c.id}&side=${side}` });
    const right = fs.existsSync(local.fsPath) ? local : q("local");
    await vscode.commands.executeCommand("vscode.diff", q("remote"), right, `${c.path}: server ↔ yours (conflict #${c.id})`);
  }

  // conflictContent backs the rex-conflict: scheme used by compare.
  async conflictContent(uri: vscode.Uri): Promise<string> {
    const p = new URLSearchParams(uri.query);
    if (!this.cfg) {
      return "";
    }
    const res = await runRex(
      this.cfg.bin,
      ["conflicts", "--id", p.get("id") ?? "0", "--side", p.get("side") ?? "remote", ...connArgs(this.cfg)],
      { env: rexEnv(this.cfg), timeoutMs: 20_000 }
    );
    return res.code === 0 ? res.stdout : `# Could not fetch this side of the conflict\n\n${res.stderr || res.error}`;
  }

  // revertTo rolls the whole report back to right after entry's change (or
  // right before it, undoing it and everything since), gated by rex-svr's
  // admin password. It previews first (every file that
  // would change, diffable, plus the target commit and git log) and only
  // applies that exact plan once confirmed; the server refuses it if
  // anything moved in between.
  async revertTo(entry: RecentEntry): Promise<void> {
    const cfg = this.cfg;
    if (!cfg) {
      return;
    }
    const mode = await vscode.window.showQuickPick(
      [
        { label: "$(discard) Undo this change and everything after it", before: true },
        { label: "$(history) Revert to right after this change", before: false },
      ],
      { placeHolder: `${entry.path} rev ${entry.revision} by ${entry.modifiedBy}` }
    );
    if (!mode) {
      return;
    }
    const target = `${mode.before ? "before " : ""}${entry.path} rev ${entry.revision}`;
    const admin = await this.admin();
    if (!admin) {
      return;
    }
    const env = { ...rexEnv(cfg), REX_ADMIN_PASSWORD: admin };
    const run = (plan?: string) =>
      runRex(cfg.bin, [...revertArgs(entry.path, entry.revision, mode.before, plan), ...connArgs(cfg)], { env, timeoutMs: 60_000 });

    const res = await run();
    if (res.code !== 0) {
      this.adminFailed("revert preview", res.stderr.trim() || res.error || "");
      return;
    }
    const p = JSON.parse(res.stdout) as RevertPreview;
    if (p.ops.length === 0) {
      void vscode.window.showInformationMessage("Report Rex: the report already matches that change; nothing to revert.");
      return;
    }
    this.revertPreview = p;

    type Pick = vscode.QuickPickItem & { run?: () => Promise<void>; apply?: boolean };
    const commit = p.commit ? p.commit.split("\t") : undefined;
    const picks: Pick[] = [
      { label: "$(history) Revert", description: `${p.ops.length} file(s), recorded as a new commit`, apply: true },
      {
        label: "$(git-commit) Selected change's commit",
        description: commit ? `${commit[0].slice(0, 10)} ${commit[1] ?? ""}` : "revision was folded into a later commit",
        run: async () => this.showGitLog(p),
      },
      { label: "$(list-flat) Show git log", description: "recent server history", run: async () => this.showGitLog(p) },
      { label: "Changes", kind: vscode.QuickPickItemKind.Separator },
      ...p.ops.map((op) => ({
        label: `$(${op.action === "delete" ? "trash" : "diff"}) ${op.path}`,
        description: op.action === "delete" ? "deleted" : `restored to rev ${op.target_revision}`,
        run: () => this.revertDiff(op.path),
      })),
    ];
    for (;;) {
      const pick = await vscode.window.showQuickPick(picks, {
        placeHolder: `Revert the report to ${target}? Pick a file to diff it.`,
        ignoreFocusOut: true,
      });
      if (!pick) {
        return;
      }
      if (pick.run) {
        await pick.run();
        continue;
      }
      const ok = await vscode.window.showWarningMessage(
        `Revert ${p.ops.length} file(s) to ${target} for everyone?`,
        { modal: true, detail: "History is kept: each file gets a new revision and git gets a \"Revert to\" commit." },
        "Revert"
      );
      if (ok === "Revert") {
        break;
      }
    }

    const applied = await run(p.plan);
    if (applied.code !== 0) {
      this.adminFailed("revert", applied.stderr.trim() || applied.error || "");
      return;
    }
    void vscode.window.showInformationMessage(`Report Rex: reverted ${p.ops.length} file(s) to ${target}.`);
    this.syncNow();
    await this.refresh();
  }

  // deleteFiles deletes the explorer selection (folders expanded) from
  // rex-svr for everyone, gated by the admin password. It's the deliberate
  // path for a mass delete that `rex sync` refuses to push on its own.
  // rel maps a file URI to its sync-dir-relative path, or undefined outside it.
  rel(uri: vscode.Uri): string | undefined {
    if (!this.cfg || uri.scheme !== "file") {
      return undefined;
    }
    const rel = path.relative(this.cfg.dir, uri.fsPath);
    return rel && !rel.startsWith("..") && !path.isAbsolute(rel) ? rel.split(path.sep).join("/") : undefined;
  }

  // blame fetches rex-svr's per-line blame for a sync-dir-relative path;
  // a string is the error.
  async blame(rel: string): Promise<BlameResponse | string> {
    const cfg = this.cfg;
    if (!cfg) {
      return "not configured";
    }
    const res = await runRex(cfg.bin, [...blameArgs(rel), ...connArgs(cfg)], { env: rexEnv(cfg), timeoutMs: 30_000 });
    if (res.code !== 0) {
      return res.stderr.trim() || res.error || `rex blame exited with code ${res.code}`;
    }
    return JSON.parse(res.stdout) as BlameResponse;
  }

  // showBlame is Explorer's Rex Blame: the whole file's blame in a new tab.
  async showBlame(uri: vscode.Uri): Promise<void> {
    const rel = this.rel(uri);
    if (!rel) {
      void vscode.window.showInformationMessage(`Report Rex: ${uri.fsPath} is not inside the sync folder.`);
      return;
    }
    const b = await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Window, title: `Rex Blame: ${rel}` },
      () => this.blame(rel)
    );
    if (typeof b === "string") {
      void vscode.window.showErrorMessage(`Report Rex: blame failed: ${b}`);
      return;
    }
    const doc = await vscode.workspace.openTextDocument({ content: formatBlame(b), language: "plaintext" });
    await vscode.window.showTextDocument(doc, { preview: false });
  }

  async deleteFiles(uris: vscode.Uri[]): Promise<void> {
    const cfg = this.cfg;
    if (!cfg) {
      return;
    }
    const files = new Set<string>();
    const walk = (abs: string): void => {
      if (fs.lstatSync(abs, { throwIfNoEntry: false })?.isDirectory()) {
        fs.readdirSync(abs).forEach((n) => walk(path.join(abs, n)));
      } else {
        files.add(path.relative(cfg.dir, abs));
      }
    };
    for (const u of uris) {
      const rel = path.relative(cfg.dir, u.fsPath);
      if (!rel.startsWith("..") && !path.isAbsolute(rel)) {
        walk(u.fsPath);
      }
    }
    files.delete("");
    const paths = [...files].map((p) => p.split(path.sep).join("/")).sort();
    if (paths.length === 0) {
      void vscode.window.showInformationMessage(`Report Rex: nothing selected inside the sync folder ${cfg.dir}.`);
      return;
    }
    const ok = await vscode.window.showWarningMessage(
      `Delete ${paths.length} file(s) from the report for everyone?`,
      {
        modal: true,
        detail: `${paths.slice(0, 15).join("\n")}${paths.length > 15 ? `\n… and ${paths.length - 15} more` : ""}\n\nThe server keeps their history; revert brings them back.`,
      },
      "Delete"
    );
    if (ok !== "Delete") {
      return;
    }
    const admin = await this.admin();
    if (!admin) {
      return;
    }
    // ponytail: paths go on argv; a report with tens of thousands of files would hit ARG_MAX -- switch to stdin then.
    const res = await runRex(cfg.bin, [...deleteArgs(paths), ...connArgs(cfg)], {
      env: { ...rexEnv(cfg), REX_ADMIN_PASSWORD: admin },
      timeoutMs: 60_000,
    });
    if (res.code !== 0) {
      this.adminFailed("delete", res.stderr.trim() || res.error || "");
      return;
    }
    const r = JSON.parse(res.stdout) as BulkDeleteResult;
    const skipped = r.skipped.length ? ` (${r.skipped.length} skipped: missing, excluded, or handoffs)` : "";
    void vscode.window.showInformationMessage(`Report Rex: deleted ${r.deleted.length} file(s) for everyone${skipped}.`);
    this.syncNow();
    await this.refresh();
  }

  private async showGitLog(p: RevertPreview): Promise<void> {
    this.log.appendLine(`--- rex-svr git log (reverting to ${p.before ? "before " : ""}${p.path} rev ${p.revision}) ---`);
    if (p.commit) {
      this.log.appendLine(`selected change's commit: ${p.commit.replace("\t", " ")}`);
    }
    p.git_log.forEach((l) => this.log.appendLine(l));
    this.log.show(true);
  }

  // revertDiff diffs a file as it is now against how the revert leaves it.
  private async revertDiff(rel: string): Promise<void> {
    const local = this.abs(rel);
    // plan in the URI keeps VS Code from reusing a previous preview's document.
    const plan = this.revertPreview?.plan.slice(0, 12);
    const q = (side: string) => vscode.Uri.from({ scheme: REVERT_SCHEME, path: "/" + rel, query: `side=${side}&plan=${plan}` });
    const left = local && fs.existsSync(local.fsPath) ? local : q("none");
    await vscode.commands.executeCommand("vscode.diff", left, q("target"), `${rel}: now ↔ after revert`);
  }

  // revertContent backs the rex-revert: scheme used by revertDiff.
  revertContent(uri: vscode.Uri): string {
    const op = this.revertPreview?.ops.find((o) => "/" + o.path === uri.path);
    if (!op?.content || new URLSearchParams(uri.query).get("side") !== "target") {
      return "";
    }
    return Buffer.from(op.content, "base64").toString("utf8");
  }

  dispose(): void {
    clearInterval(this.poll);
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer);
    }
    this.syncBridge?.stop();
    this.presence?.dispose();
    this.changed.dispose();
  }
}

export const CONFLICT_SCHEME = "rex-conflict";
export const REVERT_SCHEME = "rex-revert";

// adminEnabled is the report-rex.admin.enabled ("Enable Admin") setting,
// which shows the admin features at all.
function adminEnabled(): boolean {
  return vscode.workspace.getConfiguration("report-rex").get<boolean>("admin.enabled", false);
}
