import * as vscode from "vscode";
import { RemoteState, othersOn, isConcurrentEdit } from "./presenceStore";

// notifyOnOpen shows a one-time heads-up when you open a file a teammate
// already has open -- mirrors init.lua's maybe_notify_on_enter. Callers
// gate it to once per open. It says "editing" only when they are; someone
// who merely has it open is "viewing".
export function notifyOnOpen(remote: RemoteState, file: string, ownUsername: string): void {
  const others = othersOn(remote, file, ownUsername);
  if (others.length === 0) {
    return;
  }
  const editor = others.find((e) => e.state === "editing");
  const who = editor ?? others[0];
  const more = others.length > 1 ? ` (+${others.length - 1} more)` : "";
  void vscode.window.showWarningMessage(`⚠ ${who.username} is ${editor ? "editing" : "viewing"} ${file}${more}`);
}

// notifyConcurrentEdit shows a louder warning when the local user starts
// editing a file someone else is *already* editing -- mirrors init.lua's
// maybe_notify_concurrent_edit.
export function notifyConcurrentEdit(remote: RemoteState, file: string, ownUsername: string): void {
  const who = isConcurrentEdit(remote, file, ownUsername);
  if (who) {
    void vscode.window.showErrorMessage(
      `⚠ CONCURRENT EDIT: ${who} is editing ${file} too. Edits to different lines merge automatically; the same lines become a conflict.`
    );
  }
}
