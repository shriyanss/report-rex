import * as vscode from "vscode";
import { isEditing } from "./presenceStore";
import { relativeTo } from "./model";
import { RexController } from "./controller";

// RexDecorationProvider marks files in the Explorer, tabs, and quick-open:
// a red "!" for your open sync conflicts (the thing you must act on), else
// a dot for files a teammate is editing right now. Keyed by exact path
// relative to the synced directory -- no basename-collision ambiguity,
// unlike the Neovim plugin's rendered-buffer-text scan.
export class RexDecorationProvider implements vscode.FileDecorationProvider {
  private readonly emitter = new vscode.EventEmitter<vscode.Uri | vscode.Uri[] | undefined>();
  readonly onDidChangeFileDecorations = this.emitter.event;

  constructor(private readonly ctrl: RexController) {}

  provideFileDecoration(uri: vscode.Uri): vscode.FileDecoration | undefined {
    if (uri.scheme !== "file" || !this.ctrl.cfg) {
      return undefined;
    }
    const rel = relativeTo(this.ctrl.cfg.dir, uri.fsPath);
    if (!rel) {
      return undefined;
    }
    const conflict = this.ctrl.myConflicts().find((c) => c.path === rel);
    if (conflict) {
      return {
        badge: "!",
        color: new vscode.ThemeColor("problemsErrorIcon.foreground"),
        tooltip: `Sync conflict #${conflict.id}: a teammate changed this file too — resolve it in the Report Rex view`,
        propagate: true,
      };
    }
    const entry = isEditing(this.ctrl.remote, rel, this.ctrl.me);
    if (!entry) {
      return undefined;
    }
    return {
      badge: "●",
      color: new vscode.ThemeColor("problemsWarningIcon.foreground"),
      tooltip: `${entry.username} is editing this file`,
    };
  }

  refresh(): void {
    this.emitter.fire(undefined);
  }
}
