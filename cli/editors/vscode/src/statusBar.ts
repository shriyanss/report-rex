import * as vscode from "vscode";
import { statusText } from "./presenceStore";
import { relativeTo, statusBarText, statusBarTooltip } from "./model";
import { RexController } from "./controller";

// RexStatusBar is the one Report Rex status bar item: sync state, your open
// conflicts, and who else is on the active file (the Neovim plugin's
// statusline() fragment, without asking the user to wire anything in).
export class RexStatusBar {
  private readonly item = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);

  constructor(private readonly ctrl: RexController) {
    this.item.command = "workbench.view.extension.report-rex";
  }

  refresh(editor: vscode.TextEditor | undefined): void {
    if (!this.ctrl.cfg) {
      this.item.hide();
      return;
    }
    const rel = editor && relativeTo(this.ctrl.cfg.dir, editor.document.uri.fsPath);
    const presence = rel ? statusText(this.ctrl.remote, rel, this.ctrl.me) : "";
    const mine = this.ctrl.myConflicts().length;
    this.item.text = statusBarText(this.ctrl.sync, mine, presence);
    this.item.tooltip = statusBarTooltip(this.ctrl.sync, mine, this.ctrl.cfg ? `${this.ctrl.cfg.host}:${this.ctrl.cfg.port}` : "");
    this.item.backgroundColor = mine > 0 ? new vscode.ThemeColor("statusBarItem.warningBackground") : undefined;
    this.item.show();
  }

  dispose(): void {
    this.item.dispose();
  }
}
