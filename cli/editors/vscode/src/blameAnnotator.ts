import * as vscode from "vscode";
import { BlameResponse, blameAnnotation } from "./cli";
import { RexController } from "./controller";

const DWELL_MS = 5000;

// BlameAnnotator shows GitLens-style "who/when" text at the end of the
// cursor's line, but only once the cursor has rested there for 5s, so moving
// around never hammers rex-svr. A fetched blame is reused until the document
// changes or is saved.
export class BlameAnnotator implements vscode.Disposable {
  private readonly deco = vscode.window.createTextEditorDecorationType({
    after: { color: new vscode.ThemeColor("editorCodeLens.foreground"), margin: "0 0 0 3em", fontStyle: "italic" },
  });
  private readonly cache = new Map<string, BlameResponse>();
  private timer?: ReturnType<typeof setTimeout>;
  private readonly subs: vscode.Disposable[];

  constructor(private readonly ctrl: RexController) {
    this.subs = [
      vscode.window.onDidChangeTextEditorSelection((e) => this.schedule(e.textEditor)),
      vscode.window.onDidChangeActiveTextEditor((e) => e && this.schedule(e)),
      vscode.workspace.onDidChangeTextDocument((e) => this.cache.delete(e.document.uri.toString())),
      vscode.workspace.onDidSaveTextDocument((d) => this.cache.delete(d.uri.toString())),
    ];
  }

  private schedule(editor: vscode.TextEditor): void {
    clearTimeout(this.timer);
    vscode.window.visibleTextEditors.forEach((e) => e.setDecorations(this.deco, []));
    if (!vscode.workspace.getConfiguration("report-rex").get<boolean>("blame.inline", true)) {
      return;
    }
    const line = editor.selection.active.line;
    this.timer = setTimeout(() => void this.annotate(editor, line), DWELL_MS);
  }

  private async annotate(editor: vscode.TextEditor, line: number): Promise<void> {
    const doc = editor.document;
    const rel = this.ctrl.rel(doc.uri);
    // A dirty buffer's lines no longer match the server's copy.
    if (!rel || doc.isDirty || !this.ctrl.cfg?.password) {
      return;
    }
    const key = doc.uri.toString();
    let b = this.cache.get(key);
    if (!b) {
      const res = await this.ctrl.blame(rel);
      if (typeof res === "string") {
        this.ctrl.log.appendLine(`rex blame ${rel}: ${res}`);
        return;
      }
      b = res;
      this.cache.set(key, b);
    }
    const l = b.lines[line];
    // Bail if the cursor moved on while the fetch was in flight.
    if (!l || vscode.window.activeTextEditor !== editor || editor.selection.active.line !== line) {
      return;
    }
    const end = doc.lineAt(line).range.end;
    editor.setDecorations(this.deco, [{ range: new vscode.Range(end, end), renderOptions: { after: { contentText: blameAnnotation(l) } } }]);
  }

  dispose(): void {
    clearTimeout(this.timer);
    this.subs.forEach((s) => s.dispose());
    this.deco.dispose();
  }
}
