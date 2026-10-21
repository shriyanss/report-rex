import * as fs from "fs";
import * as path from "path";
import * as vscode from "vscode";
import { ConflictSummary } from "./cli";
import { RexController } from "./controller";
import { CONFLICT_KIND_LABEL, RecentEntry, parseHandoff, relativeTime, splitConflicts, teamFromPresence } from "./model";
import { configured, panelRows } from "./panelState";
import { passwordSecretKey } from "./config";

// Node is one row in any Report Rex view. conflict rides along so
// view/item/context commands (which receive the node) know what to act on.
export interface Node {
  item: vscode.TreeItem;
  children?: Node[];
  conflict?: ConflictSummary;
  change?: RecentEntry;
}

// SimpleTree is the one TreeDataProvider behind every view: each view is
// just a function that builds its nodes from the controller's state.
export class SimpleTree implements vscode.TreeDataProvider<Node> {
  private readonly emitter = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this.emitter.event;

  constructor(private readonly build: () => Node[] | Promise<Node[]>) {}

  refresh(): void {
    this.emitter.fire();
  }

  getTreeItem(n: Node): vscode.TreeItem {
    return n.item;
  }

  getChildren(n?: Node): Node[] | Promise<Node[]> {
    return n ? n.children ?? [] : this.build();
  }
}

function item(label: string, opts: Partial<vscode.TreeItem> & { icon?: string; color?: string } = {}): vscode.TreeItem {
  const { icon, color, ...rest } = opts;
  const it = new vscode.TreeItem(label, vscode.TreeItemCollapsibleState.None);
  Object.assign(it, rest);
  if (icon) {
    it.iconPath = new vscode.ThemeIcon(icon, color ? new vscode.ThemeColor(color) : undefined);
  }
  return it;
}

function group(label: string, children: Node[], icon: string, description?: string): Node {
  const it = item(label, { icon, description });
  it.collapsibleState = vscode.TreeItemCollapsibleState.Expanded;
  return { item: it, children };
}

function openCommand(uri: vscode.Uri | undefined): vscode.Command | undefined {
  return uri ? { command: "vscode.open", title: "Open", arguments: [uri] } : undefined;
}

const REPORT_ACTIONS: [string, string, string][] = [
  ["Build Draft", "report-rex.buildDraft", "file-pdf"],
  ["Build Final", "report-rex.buildFinal", "verified"],
  ["Add Finding", "report-rex.addFinding", "bug"],
  ["Add Appendix", "report-rex.addAppendix", "note"],
  ["Add Narrative Section", "report-rex.addNarrative", "book"],
];

// reportNodes lists the report actions, or nothing (so the view's welcome
// content, with its Initialize Report button, shows) outside a report.
export function reportNodes(isProject: boolean): Node[] {
  return isProject
    ? REPORT_ACTIONS.map(([label, command, icon]) => ({ item: item(label, { icon, command: { command, title: label } }) }))
    : [];
}

export async function syncNodes(ctrl: RexController, context: vscode.ExtensionContext): Promise<Node[]> {
  const cfg = vscode.workspace.getConfiguration("report-rex");
  const host = cfg.get<string>("host", "");
  const username = cfg.get<string>("username", "");
  if (!configured({ host, username })) {
    return []; // empty -> the viewsWelcome setup walkthrough shows instead
  }
  const { mine, others } = splitConflicts(ctrl.conflicts, ctrl.me || username);
  const rows = panelRows({
    sync: ctrl.sync,
    syncEnabled: ctrl.syncRunning,
    presenceConnected: ctrl.presenceConnected,
    health: ctrl.health,
    healthCheckedAt: ctrl.healthCheckedAt,
    myConflicts: mine.length,
    otherConflicts: others.length,
    host,
    port: cfg.get<number>("port", 9443),
    username,
    dir: cfg.get<string>("dir", "") || vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || "",
    passwordSet: !!(await context.secrets.get(passwordSecretKey())) || !!process.env.REX_PASSWORD,
    caCert: cfg.get<string>("caCert", ""),
    insecure: cfg.get<boolean>("insecure", false),
    lintEnabled: cfg.get<boolean>("lint.enabled", true),
  });
  return rows.map((r) => ({
    item: item(r.label, {
      description: r.description,
      tooltip: r.tooltip ?? r.description,
      icon: r.icon,
      color: r.color,
      command: r.command ? { command: r.command, title: r.label } : undefined,
    }),
  }));
}

function conflictNode(ctrl: RexController, c: ConflictSummary, mine: boolean): Node {
  const pending = ctrl.pendingMerges.get(c.path) === c.id;
  const who = mine ? "" : ` · ${c.local_username}`;
  const it = item(c.path, {
    description: `${CONFLICT_KIND_LABEL[c.kind] ?? c.kind}${who} · ${relativeTime(c.created_at)}${pending ? " · merging" : ""}`,
    tooltip:
      `Conflict #${c.id} on ${c.path} (${c.kind})\n` +
      (mine
        ? "Your change was kept locally; the server has a teammate's newer version.\nClick to compare, or use the buttons to resolve."
        : `${c.local_username}'s change collided with the server's copy. They resolve it from their side.`),
    icon: pending ? "git-merge" : mine ? "warning" : "person",
    color: mine ? "problemsWarningIcon.foreground" : undefined,
    contextValue: pending ? "conflict.pending" : mine ? "conflict.mine" : "conflict.other",
    command: { command: "report-rex.conflict.compare", title: "Compare", arguments: [{ conflict: c }] },
  });
  it.resourceUri = ctrl.abs(c.path);
  return { item: it, conflict: c };
}

export function conflictNodes(ctrl: RexController): Node[] {
  const { mine, others } = splitConflicts(ctrl.conflicts, ctrl.me);
  const out: Node[] = [];
  if (mine.length) {
    out.push(group("Yours", mine.map((c) => conflictNode(ctrl, c, true)), "account", `${mine.length} to resolve`));
  }
  if (others.length) {
    out.push(group("Teammates'", others.map((c) => conflictNode(ctrl, c, false)), "organization", String(others.length)));
  }
  return out;
}

export function changeNodes(ctrl: RexController): Node[] {
  const now = Date.now();
  return ctrl.recent.map((e) => {
    const who = e.modifiedBy === ctrl.me ? "you" : e.modifiedBy;
    const uri = e.deleted ? undefined : ctrl.abs(e.path);
    const it = item(e.path, {
      description: `${who} · ${relativeTime(e.at, now)}${e.deleted ? " · deleted" : ""}`,
      tooltip: `${e.path}\n${e.deleted ? "Deleted" : `Revision ${e.revision}`} by ${e.modifiedBy} at ${e.at}`,
      icon: e.deleted ? "trash" : e.modifiedBy === ctrl.me ? "arrow-up" : "arrow-down",
      command: openCommand(uri),
      contextValue: "change",
    });
    if (uri) {
      it.resourceUri = uri;
    }
    return { item: it, change: e };
  });
}

export function teamNodes(ctrl: RexController): Node[] {
  return teamFromPresence(ctrl.remote, ctrl.me).map((m) => {
    const editing = m.files.filter((f) => f.state === "editing").length;
    const files = m.files.map((f) => {
      const uri = ctrl.abs(f.file);
      const it = item(f.file, {
        description: f.state === "editing" ? (f.dirty ? "editing (unsaved)" : "editing") : "viewing",
        icon: f.state === "editing" ? "edit" : "eye",
        command: openCommand(uri),
      });
      return { item: it };
    });
    return group(m.username, files, "account", editing ? `editing ${editing}` : `viewing ${m.files.length}`);
  });
}

// handoffNodes lists handoffs addressed to you and ones you gave out, read
// straight from the synced handoffs/<assignee>/ tree.
export function handoffNodes(ctrl: RexController): Node[] {
  const dir = ctrl.cfg?.dir;
  const me = ctrl.me;
  if (!dir || !me) {
    return [];
  }
  const root = path.join(dir, "handoffs");
  const forMe: Node[] = [];
  const fromMe: Node[] = [];
  let assignees: string[] = [];
  try {
    assignees = fs.readdirSync(root);
  } catch {
    return [];
  }
  for (const who of assignees) {
    let files: string[] = [];
    try {
      files = fs.readdirSync(path.join(root, who)).filter((f) => f.endsWith(".md"));
    } catch {
      continue;
    }
    for (const f of files) {
      const rel = `handoffs/${who}/${f}`;
      let h;
      try {
        h = parseHandoff(rel, fs.readFileSync(path.join(root, who, f), "utf8"));
      } catch {
        continue;
      }
      if (h.status === "resolved") {
        continue;
      }
      const uri = ctrl.abs(rel);
      if (h.to === me) {
        forMe.push({ item: item(h.title, { description: `from ${h.from || "?"}`, icon: "inbox", command: openCommand(uri), tooltip: rel }) });
      } else if (h.from === me) {
        fromMe.push({ item: item(h.title, { description: `to ${h.to}`, icon: "send", command: openCommand(uri), tooltip: rel }) });
      }
    }
  }
  const out: Node[] = [];
  if (forMe.length) {
    out.push(group("For you", forMe, "inbox", String(forMe.length)));
  }
  if (fromMe.length) {
    out.push(group("From you", fromMe, "send", String(fromMe.length)));
  }
  return out;
}
