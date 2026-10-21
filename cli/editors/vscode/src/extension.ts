import * as vscode from "vscode";
import { resolveLintConfig, passwordSecretKey } from "./config";
import { RexController, CONFLICT_SCHEME, REVERT_SCHEME, Resolution } from "./controller";
import { RexDecorationProvider } from "./decorationProvider";
import { BlameAnnotator } from "./blameAnnotator";
import { RexStatusBar } from "./statusBar";
import { Node, SimpleTree, changeNodes, conflictNodes, handoffNodes, reportNodes, syncNodes, teamNodes } from "./views";
import { notifyOnOpen, notifyConcurrentEdit } from "./notifications";
import { runLint } from "./lintBridge";
import {
  Issue,
  LINTED_ARTIFACTS_GLOB,
  LINT_ON_EDIT_DELAY_MS,
  ProjectOverride,
  diagnosticLine,
  effectiveLintEnabled,
  groupIssuesByFile,
  titleInsertion,
} from "./lint";
import { ConflictSummary, MIN_REX_VERSION, rexVersionOk, runRex } from "./cli";
import { hasConflictMarkers, relativeTo, splitConflicts } from "./model";
import { addSection, buildReport, initReport } from "./report";

const HEARTBEAT_MS = 5000;
const RELEASES_URL = "https://github.com/shriyanss/report-rex/releases";

// Settings that change how we reach rex-svr: any of them restarts the
// bridges. Lint settings deliberately don't.
const CONNECTION_KEYS = ["host", "port", "username", "dir", "bin", "caCert", "insecure"];

const LINT_SEVERITY: Record<Issue["severity"], vscode.DiagnosticSeverity> = {
  error: vscode.DiagnosticSeverity.Error,
  warning: vscode.DiagnosticSeverity.Warning,
};

// isRexProject auto-detects a rex-report project the same way this
// extension's own activationEvents do (workspaceContains:**/index.md).
async function isRexProject(folder: vscode.WorkspaceFolder): Promise<boolean> {
  const found = await vscode.workspace.findFiles(new vscode.RelativePattern(folder, "**/index.md"), null, 1);
  return found.length > 0;
}

// toDiagnostics converts one file's issues into vscode.Diagnostics. An issue
// with a line (a chart directive, a connections.json entry) underlines that
// whole line; a whole-file issue sits at the top of the file.
function toDiagnostics(issues: Issue[]): vscode.Diagnostic[] {
  return issues.map((issue) => {
    const line = diagnosticLine(issue);
    const diagnostic = new vscode.Diagnostic(
      issue.line ? new vscode.Range(line, 0, line, Number.MAX_SAFE_INTEGER) : new vscode.Range(0, 0, 0, 0),
      `${issue.message} (${issue.check})`,
      LINT_SEVERITY[issue.severity]
    );
    diagnostic.source = "rex report validate";
    diagnostic.code = issue.check;
    return diagnostic;
  });
}

// TitleQuickFix offers to insert a `title:` line for a title_missing
// diagnostic (#58).
class TitleQuickFix implements vscode.CodeActionProvider {
  provideCodeActions(doc: vscode.TextDocument, _range: vscode.Range, ctx: vscode.CodeActionContext): vscode.CodeAction[] {
    const diagnostic = ctx.diagnostics.find((d) => d.code === "title_missing");
    if (!diagnostic) {
      return [];
    }
    const { line, text } = titleInsertion(doc.getText());
    const action = new vscode.CodeAction("Add a title to the frontmatter", vscode.CodeActionKind.QuickFix);
    action.edit = new vscode.WorkspaceEdit();
    action.edit.insert(doc.uri, new vscode.Position(line, 0), text);
    action.diagnostics = [diagnostic];
    action.isPreferred = true;
    return [action];
  }
}

// conflictArg accepts what a conflict command can be invoked with: a tree
// node (view buttons), a ConflictSummary (notifications), or nothing
// (Command Palette -> pick one of yours).
async function conflictArg(ctrl: RexController, arg?: Node | ConflictSummary): Promise<ConflictSummary | undefined> {
  const c = (arg as Node | undefined)?.conflict ?? (arg && "id" in arg ? (arg as ConflictSummary) : undefined);
  if (c) {
    return c;
  }
  const mine = ctrl.myConflicts();
  if (mine.length === 0) {
    void vscode.window.showInformationMessage("Report Rex: you have no open conflicts.");
    return undefined;
  }
  const pick = await vscode.window.showQuickPick(
    mine.map((x) => ({ label: x.path, description: `#${x.id} ${x.kind}`, c: x })),
    { placeHolder: "Which conflict?" }
  );
  return pick?.c;
}

// The test API integration tests reach via extension.exports.
export interface RexApi {
  controller: RexController;
  views: { sync(): Promise<Node[]>; conflicts(): Node[]; changes(): Node[]; team(): Node[]; handoffs(): Node[] };
}

// activate wires the presence/sync bridges to VS Code's own document and
// editor-lifecycle events -- the native equivalent of init.lua's
// BufEnter/InsertEnter/TextChanged/BufLeave/VimLeavePre autocommands. Any
// bridge/connection failure just means less info is shown; it never
// blocks editing.
export function activate(context: vscode.ExtensionContext): RexApi {
  const outputChannel = vscode.window.createOutputChannel("Report Rex");
  const ctrl = new RexController(context, outputChannel);
  const tracked = new Map<string, { state: "viewing" | "editing"; dirty: boolean }>();
  const notifiedOnOpen = new Set<string>();

  const decorations = new RexDecorationProvider(ctrl);
  const statusBar = new RexStatusBar(ctrl);
  const diagnostics = vscode.languages.createDiagnosticCollection("report-rex");
  const views = {
    sync: () => syncNodes(ctrl, context),
    conflicts: () => conflictNodes(ctrl),
    changes: () => changeNodes(ctrl),
    team: () => teamNodes(ctrl),
    handoffs: () => handoffNodes(ctrl),
  };
  const trees = {
    sync: new SimpleTree(views.sync),
    conflicts: new SimpleTree(views.conflicts),
    changes: new SimpleTree(views.changes),
    team: new SimpleTree(views.team),
    handoffs: new SimpleTree(views.handoffs),
  };
  // Kept out of `trees`: render() fires on every heartbeat, and this one
  // only changes when an index.md appears or goes away.
  const reportTree = new SimpleTree(async () => {
    const folder = vscode.workspace.workspaceFolders?.[0];
    return reportNodes(!!folder && (await isRexProject(folder)));
  });
  const indexWatcher = vscode.workspace.createFileSystemWatcher("**/index.md", false, true, false);
  indexWatcher.onDidCreate(() => reportTree.refresh());
  indexWatcher.onDidDelete(() => reportTree.refresh());
  // Debounced: one `rex ingest` run can rewrite several artifact files at once.
  let artifactLintTimer: ReturnType<typeof setTimeout> | undefined;
  const lintSoon = () => {
    clearTimeout(artifactLintTimer);
    artifactLintTimer = setTimeout(() => void runLintForWorkspace(), 300);
  };
  const artifactWatcher = vscode.workspace.createFileSystemWatcher(LINTED_ARTIFACTS_GLOB);
  artifactWatcher.onDidCreate(lintSoon);
  artifactWatcher.onDidChange(lintSoon);
  artifactWatcher.onDidDelete(lintSoon);
  const conflictsView = vscode.window.createTreeView("reportRex.conflicts", { treeDataProvider: trees.conflicts });

  // rel maps a document to its path relative to the synced directory
  // (which presence and conflicts are keyed by), or null outside it.
  const rel = (uri: vscode.Uri): string | null =>
    uri.scheme === "file" ? relativeTo(ctrl.cfg?.dir || resolveLintConfig().dir, uri.fsPath) : null;

  function render(): void {
    for (const t of Object.values(trees)) {
      t.refresh();
    }
    const mine = splitConflicts(ctrl.conflicts, ctrl.me).mine.length;
    conflictsView.badge = mine ? { value: mine, tooltip: `${mine} conflict${mine === 1 ? "" : "s"} to resolve` } : undefined;
    decorations.refresh();
    statusBar.refresh(vscode.window.activeTextEditor);
  }
  ctrl.onDidChange(render);

  // Each run supersedes the ones before it; a slower earlier run that
  // finishes later mustn't overwrite newer diagnostics.
  let lintRun = 0;
  let editLintTimer: ReturnType<typeof setTimeout> | undefined;
  // lintSoonOnEdit re-lints as you type (#58), so e.g. a missing quote shows
  // up before saving.
  const lintSoonOnEdit = () => {
    clearTimeout(editLintTimer);
    editLintTimer = setTimeout(() => void runLintForWorkspace(), LINT_ON_EDIT_DELAY_MS);
  };
  context.subscriptions.push({ dispose: () => clearTimeout(editLintTimer) });

  // unsavedMarkdown is every dirty markdown buffer, linted in place of its
  // file on disk; undefined when there are none.
  function unsavedMarkdown(): Record<string, string> | undefined {
    const dirty = vscode.workspace.textDocuments.filter(
      (doc) => doc.isDirty && doc.languageId === "markdown" && doc.uri.scheme === "file"
    );
    return dirty.length ? Object.fromEntries(dirty.map((doc) => [doc.uri.fsPath, doc.getText()])) : undefined;
  }

  async function runLintForWorkspace(): Promise<void> {
    const run = ++lintRun;
    const cfg = resolveLintConfig();
    const folder = vscode.workspace.workspaceFolders?.[0];
    if (!cfg.dir || !folder) {
      diagnostics.clear();
      return;
    }
    const active = effectiveLintEnabled(cfg.enabled, cfg.projectOverride, await isRexProject(folder));
    if (!active) {
      diagnostics.clear();
      return;
    }
    const overlay = unsavedMarkdown();
    const result = await runLint(cfg.dir, cfg.bin, overlay);
    if (run !== lintRun) {
      return;
    }
    // An unsaved-buffer run that fails (e.g. a rex too old for --stdin)
    // keeps the last diagnostics rather than wiping them on every keystroke.
    if (overlay && (!result || "error" in result)) {
      return;
    }
    diagnostics.clear();
    if (!result || "error" in result) {
      return;
    }
    for (const [file, issues] of groupIssuesByFile(result.issues)) {
      diagnostics.set(vscode.Uri.joinPath(folder.uri, file), toDiagnostics(issues));
    }
  }

  function sendUpdate(file: string, state: "viewing" | "editing", dirty: boolean): void {
    tracked.set(file, { state, dirty });
    ctrl.sendPresence({ op: "update", file, state, dirty });
  }

  function sendRelease(file: string): void {
    if (!tracked.has(file)) {
      return;
    }
    ctrl.sendPresence({ op: "release", file });
    tracked.delete(file);
    notifiedOnOpen.delete(file);
  }

  // setWorkspaceSetting prompts for a report-rex.<key> value and writes it
  // to workspace (not user/global) settings -- every report-rex.* value is
  // scoped to this workspace, the password included (see passwordSecretKey).
  // .vscode/ is never synced, so these stay yours.
  async function setWorkspaceSetting(
    key: string,
    prompt: string,
    parse?: (raw: string) => string | number | undefined
  ): Promise<void> {
    const cfg = vscode.workspace.getConfiguration("report-rex");
    const current = cfg.get<string | number>(key, "");
    const raw = await vscode.window.showInputBox({
      prompt,
      value: current ? String(current) : "",
      validateInput: parse ? (v) => (v && parse(v) === undefined ? "Invalid value" : null) : undefined,
    });
    if (raw === undefined) {
      return;
    }
    const value = parse ? parse(raw) : raw;
    if (value === undefined) {
      return;
    }
    await cfg.update(key, value, vscode.ConfigurationTarget.Workspace);
  }

  const parsePort = (raw: string): number | undefined => {
    const n = Number(raw);
    return Number.isInteger(n) && n > 0 && n <= 65535 ? n : undefined;
  };

  let reloadTimer: ReturnType<typeof setTimeout> | undefined;
  const reloadSoon = () => {
    if (reloadTimer) {
      clearTimeout(reloadTimer);
    }
    reloadTimer = setTimeout(() => void ctrl.reload(), 300);
  };

  const resolveWith = (how: Resolution) => async (arg?: Node | ConflictSummary) => {
    const c = await conflictArg(ctrl, arg);
    if (c) {
      await ctrl.resolve(c, how);
    }
  };

  // submitPendingMerge finishes a "Merge in editor": once the markers are
  // gone, `rex resolve --merge` again submits the file as-is.
  async function submitPendingMerge(doc: vscode.TextDocument, ask: boolean): Promise<void> {
    const file = rel(doc.uri);
    const id = file ? ctrl.pendingMerges.get(file) : undefined;
    if (!file || id === undefined) {
      return;
    }
    if (hasConflictMarkers(doc.getText())) {
      if (!ask) {
        void vscode.window.showWarningMessage(`Report Rex: ${file} still has conflict markers; resolve every block first.`);
      }
      return;
    }
    if (ask) {
      const pick = await vscode.window.showInformationMessage(
        `Report Rex: submit ${file} as the resolution of conflict #${id}?`,
        "Submit",
        "Not yet"
      );
      if (pick !== "Submit") {
        return;
      }
    }
    const c = ctrl.conflicts.find((x) => x.id === id) ?? {
      id,
      path: file,
      kind: "divergent_edit",
      remote_revision: 0,
      local_username: ctrl.me,
      created_at: "",
    };
    await ctrl.resolve(c, "merge");
  }

  context.subscriptions.push(
    outputChannel,
    ctrl,
    diagnostics,
    conflictsView,
    vscode.window.registerFileDecorationProvider(decorations),
    indexWatcher,
    artifactWatcher,
    vscode.window.registerTreeDataProvider("reportRex.report", reportTree),
    vscode.window.registerTreeDataProvider("reportRex.sync", trees.sync),
    vscode.window.registerTreeDataProvider("reportRex.changes", trees.changes),
    vscode.window.registerTreeDataProvider("reportRex.team", trees.team),
    vscode.window.registerTreeDataProvider("reportRex.handoffs", trees.handoffs),
    vscode.workspace.registerTextDocumentContentProvider(CONFLICT_SCHEME, {
      provideTextDocumentContent: (uri) => ctrl.conflictContent(uri),
    }),
    vscode.workspace.registerTextDocumentContentProvider(REVERT_SCHEME, {
      provideTextDocumentContent: (uri) => ctrl.revertContent(uri),
    }),
    // Explorer passes the clicked item, then the whole multi-selection.
    new BlameAnnotator(ctrl),
    vscode.commands.registerCommand("report-rex.blame", async (uri?: vscode.Uri) => {
      const target = uri ?? vscode.window.activeTextEditor?.document.uri;
      if (target) {
        await ctrl.showBlame(target);
      }
    }),
    vscode.commands.registerCommand("report-rex.deleteFiles", async (uri?: vscode.Uri, uris?: vscode.Uri[]) => {
      const selected = uris?.length ? uris : uri ? [uri] : [];
      if (selected.length) {
        await ctrl.deleteFiles(selected);
      }
    }),
    vscode.commands.registerCommand("report-rex.revertToChange", async (n?: Node) => {
      if (n?.change) {
        await ctrl.revertTo(n.change);
      }
    }),
    { dispose: () => statusBar.dispose() },
    vscode.commands.registerCommand("report-rex.setPassword", async () => {
      const pw = await vscode.window.showInputBox({ prompt: "rex-svr shared password", password: true });
      if (pw !== undefined) {
        await context.secrets.store(passwordSecretKey(), pw);
        await ctrl.reload();
      }
    }),
    vscode.commands.registerCommand("report-rex.startSync", () => ctrl.startSync()),
    vscode.commands.registerCommand("report-rex.stopSync", () => ctrl.stopSync()),
    vscode.commands.registerCommand("report-rex.toggleSync", () => (ctrl.syncRunning ? ctrl.stopSync() : ctrl.startSync())),
    vscode.commands.registerCommand("report-rex.syncNow", () => {
      if (!ctrl.syncRunning) {
        return ctrl.startSync();
      }
      ctrl.syncNow();
    }),
    vscode.commands.registerCommand("report-rex.testConnection", () => ctrl.testConnection(true)),
    vscode.commands.registerCommand("report-rex.refresh", async () => {
      await ctrl.refresh();
      render();
    }),
    vscode.commands.registerCommand("report-rex.showLog", () => outputChannel.show()),
    vscode.commands.registerCommand("report-rex.setHost", () => setWorkspaceSetting("host", "rex-svr host")),
    vscode.commands.registerCommand("report-rex.setPort", () =>
      setWorkspaceSetting("port", "rex-svr port (1-65535)", parsePort)
    ),
    vscode.commands.registerCommand("report-rex.setUsername", () => setWorkspaceSetting("username", "Username")),
    vscode.commands.registerCommand("report-rex.setDir", () =>
      setWorkspaceSetting("dir", "Workspace directory to sync (default: the first workspace folder)")
    ),
    vscode.commands.registerCommand("report-rex.configureTls", async () => {
      const choice = await vscode.window.showQuickPick(
        [
          { label: "$(lock) Pin the server's certificate…", description: "Recommended: the .crt file rex-svr printed on startup", id: "pin" },
          { label: "$(unlock) Skip verification (insecure)", description: "Only on a network you trust", id: "insecure" },
          { label: "$(clear-all) Clear", description: "Remove both settings", id: "clear" },
        ],
        { placeHolder: "Report Rex: how should rex trust rex-svr's self-signed certificate?" }
      );
      const cfg = vscode.workspace.getConfiguration("report-rex");
      const ws = vscode.ConfigurationTarget.Workspace;
      if (choice?.id === "pin") {
        const files = await vscode.window.showOpenDialog({
          canSelectMany: false,
          openLabel: "Trust this certificate",
          filters: { Certificates: ["crt", "pem", "cer"], "All files": ["*"] },
        });
        if (files?.[0]) {
          await cfg.update("caCert", files[0].fsPath, ws);
          await cfg.update("insecure", false, ws);
        }
      } else if (choice?.id === "insecure") {
        await cfg.update("insecure", true, ws);
      } else if (choice?.id === "clear") {
        await cfg.update("caCert", undefined, ws);
        await cfg.update("insecure", undefined, ws);
      }
    }),
    vscode.commands.registerCommand("report-rex.toggleLint", async () => {
      const cfg = vscode.workspace.getConfiguration("report-rex");
      const enabled = cfg.get<boolean>("lint.enabled", true);
      await cfg.update("lint.enabled", !enabled, vscode.ConfigurationTarget.Workspace);
      vscode.window.setStatusBarMessage(`Report Rex: linter ${!enabled ? "enabled" : "disabled"}.`, 3000);
    }),
    vscode.commands.registerCommand("report-rex.setProjectOverride", async () => {
      const choice = await vscode.window.showQuickPick(
        [
          { label: "auto", description: "Detect via index.md (default)" },
          { label: "on", description: "Always treat this workspace as a rex report project" },
          { label: "off", description: "Never lint this workspace" },
        ],
        { placeHolder: "Report Rex: is this a rex report project?" }
      );
      if (choice) {
        await vscode.workspace
          .getConfiguration("report-rex")
          .update("projectOverride", choice.label as ProjectOverride, vscode.ConfigurationTarget.Workspace);
      }
    }),
    vscode.commands.registerCommand("report-rex.conflict.compare", async (arg?: Node | ConflictSummary) => {
      const c = await conflictArg(ctrl, arg);
      if (c) {
        await ctrl.compare(c);
      }
    }),
    vscode.commands.registerCommand("report-rex.conflict.keepMine", resolveWith("local")),
    vscode.commands.registerCommand("report-rex.conflict.takeServer", resolveWith("remote")),
    vscode.commands.registerCommand("report-rex.conflict.merge", resolveWith("merge")),
    vscode.commands.registerCommand("report-rex.conflict.keepBoth", resolveWith("keep-both")),
    vscode.commands.registerCommand("report-rex.conflict.finishMerge", async (arg?: Node | ConflictSummary) => {
      const c = await conflictArg(ctrl, arg);
      const uri = c && ctrl.abs(c.path);
      if (uri) {
        const doc = await vscode.workspace.openTextDocument(uri);
        await doc.save();
        await submitPendingMerge(doc, false);
      }
    }),
    vscode.commands.registerCommand("report-rex.conflict.resolve", async (arg?: Node | ConflictSummary) => {
      const c = await conflictArg(ctrl, arg);
      if (!c) {
        return;
      }
      const deleteEdit = c.kind === "delete_edit";
      const picks: { label: string; description: string; how?: Resolution; compare?: boolean }[] = [
        { label: "$(diff) Compare first", description: "Open the server's copy next to yours", compare: true },
        { label: "$(cloud-upload) Keep mine", description: deleteEdit ? "Restore/keep your side over the deletion" : "Overwrite the server with your version", how: "local" },
        { label: "$(cloud-download) Take the server's", description: deleteEdit ? "Let the deletion stand" : "Discard your change", how: "remote" },
      ];
      if (!deleteEdit) {
        picks.push(
          { label: "$(git-merge) Merge in editor", description: "Auto-merge, or mark overlapping lines for you to pick", how: "merge" },
          { label: "$(files) Keep both", description: "Server keeps theirs; yours is saved as a .conflict copy", how: "keep-both" }
        );
      }
      const pick = await vscode.window.showQuickPick(picks, { placeHolder: `Resolve conflict #${c.id} on ${c.path}` });
      if (pick?.compare) {
        await ctrl.compare(c);
      } else if (pick?.how) {
        await ctrl.resolve(c, pick.how);
      }
    }),
    vscode.commands.registerCommand("report-rex.initReport", () => initReport(outputChannel)),
    vscode.commands.registerCommand("report-rex.buildDraft", () => buildReport(false, outputChannel)),
    vscode.commands.registerCommand("report-rex.buildFinal", () => buildReport(true, outputChannel)),
    vscode.commands.registerCommand("report-rex.addFinding", () => addSection("vuln", outputChannel)),
    vscode.commands.registerCommand("report-rex.addAppendix", () => addSection("appendix", outputChannel)),
    vscode.commands.registerCommand("report-rex.addNarrative", () => addSection("narrative", outputChannel)),
    vscode.languages.registerCodeActionsProvider({ language: "markdown", scheme: "file" }, new TitleQuickFix(), {
      providedCodeActionKinds: [vscode.CodeActionKind.QuickFix],
    }),
    vscode.workspace.onDidSaveTextDocument((doc) => {
      if (doc.languageId === "markdown") {
        void runLintForWorkspace();
      }
      const file = rel(doc.uri);
      const t = file && tracked.get(file);
      if (file && t) {
        sendUpdate(file, t.state, false);
      }
      void submitPendingMerge(doc, true);
    }),
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (!e.affectsConfiguration("report-rex")) {
        return;
      }
      if (CONNECTION_KEYS.some((k) => e.affectsConfiguration(`report-rex.${k}`))) {
        reloadSoon();
      } else if (e.affectsConfiguration("report-rex.sync.enabled")) {
        ctrl.onSyncSettingChanged();
      }
      if (e.affectsConfiguration("report-rex.admin.enabled")) {
        void ctrl.onAdminSettingChanged();
      }
      if (e.affectsConfiguration("report-rex.lint") || e.affectsConfiguration("report-rex.projectOverride")) {
        void runLintForWorkspace();
      }
      render();
    }),
    vscode.window.onDidChangeActiveTextEditor((editor) => {
      statusBar.refresh(editor);
      const file = editor && rel(editor.document.uri);
      if (!editor || !file) {
        return;
      }
      const already = tracked.get(file);
      if (!already || already.state !== "editing") {
        sendUpdate(file, "viewing", editor.document.isDirty);
      }
      if (!notifiedOnOpen.has(file)) {
        notifiedOnOpen.add(file);
        notifyOnOpen(ctrl.remote, file, ctrl.me);
      }
    }),
    vscode.workspace.onDidChangeTextDocument((e) => {
      // A change that leaves the document clean is VS Code reloading it
      // from disk -- i.e. sync just pulled a teammate's edit. That's not you
      // editing, so it mustn't mark you "editing" or warn about a clash.
      if (e.contentChanges.length === 0 || !e.document.isDirty) {
        return;
      }
      if (e.document.languageId === "markdown") {
        lintSoonOnEdit();
      }
      const file = rel(e.document.uri);
      if (!file) {
        return;
      }
      const wasEditing = tracked.get(file)?.state === "editing";
      sendUpdate(file, "editing", true);
      if (!wasEditing) {
        notifyConcurrentEdit(ctrl.remote, file, ctrl.me);
      }
    }),
    vscode.workspace.onDidCloseTextDocument((doc) => {
      const file = rel(doc.uri);
      if (file) {
        sendRelease(file);
      }
    }),
    vscode.workspace.onDidCreateFiles(() => trees.handoffs.refresh()),
    vscode.workspace.onDidDeleteFiles(() => trees.handoffs.refresh())
  );

  const heartbeat = setInterval(() => {
    for (const [file, t] of tracked) {
      ctrl.sendPresence({ op: "update", file, state: t.state, dirty: t.dirty });
    }
    // Relative times ("2m ago") in the views drift otherwise.
    trees.changes.refresh();
  }, HEARTBEAT_MS);
  context.subscriptions.push({ dispose: () => clearInterval(heartbeat) });

  void (async () => {
    await ctrl.reload();
    const activeEditor = vscode.window.activeTextEditor;
    const activeFile = activeEditor && rel(activeEditor.document.uri);
    if (activeEditor && activeFile) {
      sendUpdate(activeFile, "viewing", activeEditor.document.isDirty);
    }
    render();
  })();
  void runLintForWorkspace();
  void checkRexVersion(outputChannel);

  return { controller: ctrl, views };
}

// checkRexVersion warns once per activation if the configured rex predates
// the JSON interfaces this extension drives.
async function checkRexVersion(log: vscode.OutputChannel): Promise<void> {
  const { bin } = resolveLintConfig();
  const res = await runRex(bin, ["version"], { timeoutMs: 10_000 });
  if (res.error || res.code !== 0) {
    log.appendLine(`rex version failed: ${res.error ?? res.stderr}`);
    const pick = await vscode.window.showWarningMessage(
      `Report Rex: couldn't run "${bin}" (${res.error ?? `exit ${res.code}`}). Install rex or set report-rex.bin.`,
      "Open Settings",
      "Download rex"
    );
    if (pick === "Open Settings") {
      await vscode.commands.executeCommand("workbench.action.openWorkspaceSettings", "report-rex.bin");
    } else if (pick) {
      await vscode.env.openExternal(vscode.Uri.parse(RELEASES_URL));
    }
    return;
  }
  const version = res.stdout.trim();
  log.appendLine(`rex ${version}`);
  if (!rexVersionOk(version)) {
    const pick = await vscode.window.showWarningMessage(
      `Report Rex: this extension needs rex ${MIN_REX_VERSION} or newer for sync (found ${version}).`,
      "Download rex"
    );
    if (pick) {
      await vscode.env.openExternal(vscode.Uri.parse(RELEASES_URL));
    }
  }
}

export function deactivate(): void {
  // All cleanup runs via context.subscriptions' dispose() calls.
}
