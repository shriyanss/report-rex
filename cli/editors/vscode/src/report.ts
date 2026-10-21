import * as path from "path";
import * as vscode from "vscode";
import { resolveLintConfig } from "./config";
import { parseTemplateList, runRex } from "./cli";

// Report authoring commands: thin wrappers over `rex report build` and
// `rex report section`, so the common loop never needs a terminal.

// BUILD_DIR sits under .report-rex/, which rex sync never uploads -- a PDF
// rebuilt on every tweak would otherwise ship to the whole team each time.
const BUILD_DIR = path.join(".report-rex", "build");

export async function buildReport(final: boolean, log: vscode.OutputChannel): Promise<void> {
  const { dir, bin } = resolveLintConfig();
  if (!dir) {
    void vscode.window.showWarningMessage("Report Rex: open a report folder first.");
    return;
  }
  await vscode.workspace.saveAll(false);
  const out = path.join(dir, BUILD_DIR, "report.pdf");
  const args = ["report", "build", "--dir", dir, "--output", out, "--no-terminal-bs", ...(final ? ["--final"] : [])];
  log.appendLine(`$ ${bin} ${args.join(" ")}`);
  const res = await vscode.window.withProgress(
    {
      location: vscode.ProgressLocation.Notification,
      title: `Report Rex: building the ${final ? "final" : "draft"} report…`,
    },
    () => runRex(bin, args, { cwd: dir, timeoutMs: 15 * 60_000 })
  );
  log.appendLine(res.stdout + res.stderr);
  if (res.code !== 0) {
    const pick = await vscode.window.showErrorMessage(
      `Report Rex: build failed${res.error ? `: ${res.error}` : ""}. See the log for rex's output.`,
      "Show Log"
    );
    if (pick) {
      log.show();
    }
    return;
  }
  // Drafts are renamed report.pdf -> report-draft.pdf (see root CLAUDE.md).
  const pdf = final ? out : out.replace(/\.pdf$/, "-draft.pdf");
  const pick = await vscode.window.showInformationMessage(`Report Rex: built ${path.relative(dir, pdf)}`, "Open PDF", "Reveal");
  if (pick === "Open PDF") {
    await vscode.env.openExternal(vscode.Uri.file(pdf));
  } else if (pick === "Reveal") {
    await vscode.commands.executeCommand("revealFileInOS", vscode.Uri.file(pdf));
  }
}

export type SectionKind = "vuln" | "appendix" | "narrative";

const IMPACTS = ["Critical", "High", "Medium", "Low", "Informational"];

export async function addSection(kind: SectionKind, log: vscode.OutputChannel): Promise<void> {
  const { dir, bin } = resolveLintConfig();
  if (!dir) {
    return;
  }
  const noun = kind === "vuln" ? "finding" : kind === "appendix" ? "appendix" : "section";
  // A vuln can start from a vuln-templates/ stub, which ships its own title
  // placeholder -- so the title is only required for a blank finding.
  let template: string | undefined;
  if (kind === "vuln") {
    const list = await runRex(bin, ["report", "section", "vuln", "--list-templates"], { cwd: dir, timeoutMs: 30_000 });
    const templates = list.code === 0 ? parseTemplateList(list.stdout) : [];
    if (templates.length) {
      const pick = await vscode.window.showQuickPick(
        [{ label: "Blank finding", name: "" }, ...templates.map((t) => ({ label: t.title, description: t.name, name: t.name }))],
        { placeHolder: "Start from a template", matchOnDescription: true }
      );
      if (!pick) {
        return;
      }
      template = pick.name || undefined;
    }
  }
  const title = await vscode.window.showInputBox({
    prompt: template ? `Title of the new ${noun} (leave empty to keep the template's)` : `Title of the new ${noun}`,
    validateInput: (v) => (template || v.trim() ? null : "Required"),
  });
  if (title === undefined || (!template && !title.trim())) {
    return;
  }
  const args = ["report", "section", kind, "--dir", dir];
  if (title.trim()) {
    args.push("--title", title.trim());
  }
  if (kind === "vuln") {
    if (template) {
      args.push("--template", template);
    } else {
      const impact = await vscode.window.showQuickPick(IMPACTS, { placeHolder: "Impact (Esc to skip)" });
      if (impact) {
        args.push("--impact", impact);
      }
    }
    const file = await vscode.window.showInputBox({
      prompt: "Filename under reports/",
      placeHolder: "Leave empty for the next vuln-N.md",
      validateInput: (v) => (/[\\/]|\.\./.test(v) ? "Use a plain name like ssrf-webhook" : null),
    });
    if (file === undefined) {
      return;
    }
    if (file.trim()) {
      args.push("--file", file.trim());
    }
  } else if (kind === "narrative") {
    const name = title
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-|-$/g, "");
    args.push("--name", name);
  }
  const res = await runRex(bin, args, { cwd: dir, timeoutMs: 30_000 });
  log.appendLine(`$ ${bin} ${args.join(" ")}\n${res.stdout}${res.stderr}`);
  const created = /^Created (.+)$/m.exec(res.stdout)?.[1]?.trim();
  if (res.code !== 0 || !created) {
    void vscode.window.showErrorMessage(`Report Rex: couldn't add the ${noun}: ${(res.stderr || res.error || res.stdout).trim()}`);
    return;
  }
  const note = res.stdout.split("\n").slice(1).join(" ").trim();
  if (note) {
    void vscode.window.showInformationMessage(`Report Rex: ${note}`);
  }
  await vscode.window.showTextDocument(vscode.Uri.file(path.resolve(dir, created)));
}

const INIT_TYPES = [
  { label: "Full report", args: ["--type", "full-report"] },
  { label: "Quick-comm: critical finding", args: ["--type", "quick-comm", "--kind", "critical-finding"] },
  { label: "Quick-comm: progress update", args: ["--type", "quick-comm", "--kind", "progress-update"] },
];

// initReport scaffolds a new report into the (empty) report folder via
// `rex report init`, then opens its index.md.
export async function initReport(log: vscode.OutputChannel): Promise<void> {
  const { dir, bin } = resolveLintConfig();
  if (!dir) {
    void vscode.window.showWarningMessage("Report Rex: open a folder first.");
    return;
  }
  const type = await vscode.window.showQuickPick(INIT_TYPES, { placeHolder: "Report type" });
  if (!type) {
    return;
  }
  const required = (v: string) => (v.trim() ? null : "Required");
  const title = await vscode.window.showInputBox({ prompt: "Engagement title", validateInput: required });
  if (!title) {
    return;
  }
  const client = await vscode.window.showInputBox({ prompt: "Client name", validateInput: required });
  if (!client) {
    return;
  }
  const date = await vscode.window.showInputBox({
    prompt: "Engagement date",
    value: new Date().toISOString().slice(0, 10),
    validateInput: (v) => (/^\d{4}-\d{2}-\d{2}$/.test(v.trim()) ? null : "Use YYYY-MM-DD"),
  });
  if (!date) {
    return;
  }
  const args = ["report", "init", "--dir", dir, ...type.args, "--title", title.trim(), "--client", client.trim(), "--date", date.trim()];
  const res = await runRex(bin, args, { cwd: dir, timeoutMs: 30_000 });
  log.appendLine(`$ ${bin} ${args.join(" ")}\n${res.stdout}${res.stderr}`);
  if (res.code !== 0) {
    void vscode.window.showErrorMessage(`Report Rex: couldn't initialize the report: ${(res.stderr || res.error || res.stdout).trim()}`);
    return;
  }
  await vscode.window.showTextDocument(vscode.Uri.file(path.join(dir, "index.md")));
}
