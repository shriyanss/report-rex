// Pure logic for the lint feature (issue #26) -- no `vscode` import, so this
// stays runnable under plain `node --test`, mirroring presenceStore.ts.

// Mirrors cli/internal/report/validate.go's Issue and `rex report validate
// --json`'s envelope exactly.
export interface Issue {
  file: string;
  check: string;
  message: string;
  severity: "error" | "warning";
  line?: number; // 1-based; absent when the issue applies to the whole file
}
export interface ValidateResult {
  issues: Issue[];
  errors: number;
  warnings: number;
}
export interface ValidateError {
  error: string;
}

// parseValidateOutput decodes `rex report validate --json`'s stdout. Returns
// null for empty/malformed output -- linting is advisory, a corrupt run just
// means diagnostics aren't updated rather than an error being thrown
// (mirrors presenceBridge.ts's parseLine).
export function parseValidateOutput(stdout: string): ValidateResult | ValidateError | null {
  const trimmed = stdout.trim();
  if (!trimmed) {
    return null;
  }
  let decoded: unknown;
  try {
    decoded = JSON.parse(trimmed);
  } catch {
    return null;
  }
  if (typeof decoded !== "object" || decoded === null) {
    return null;
  }
  const obj = decoded as Record<string, unknown>;
  if (typeof obj.error === "string") {
    return { error: obj.error };
  }
  if (Array.isArray(obj.issues)) {
    return decoded as ValidateResult;
  }
  return null;
}

// groupIssuesByFile buckets issues by the (workspace-relative) file they
// belong to, preserving order within each file.
export function groupIssuesByFile(issues: Issue[]): Map<string, Issue[]> {
  const byFile = new Map<string, Issue[]>();
  for (const issue of issues) {
    const list = byFile.get(issue.file) ?? [];
    list.push(issue);
    byFile.set(issue.file, list);
  }
  return byFile;
}

// LINTED_ARTIFACTS_GLOB matches the artifact JSON files `rex report validate`
// lints alongside the markdown (issue #53), found by filename. Changes to them
// -- saved here, or rewritten by `rex ingest` -- re-run the linter.
export const LINTED_ARTIFACTS_GLOB = "**/artifacts/{assets,connections,webfindings}.json";

// diagnosticLine maps an issue to the 0-based editor line it points at: its
// 1-based `line` when validate reported one, else the top of the file.
export function diagnosticLine(issue: Issue): number {
  return issue.line && issue.line > 0 ? issue.line - 1 : 0;
}

export type ProjectOverride = "auto" | "on" | "off";

// effectiveLintEnabled combines the runtime toggle (report-rex.lint.enabled)
// with the manual project-type override (report-rex.projectOverride):
// lintEnabled is the master switch; "off" forces linting off regardless of
// detection, "on" skips auto-detection, "auto" (default) defers to whether
// index.md was actually found in the workspace folder.
export function effectiveLintEnabled(lintEnabled: boolean, override: ProjectOverride, isRexProject: boolean): boolean {
  if (!lintEnabled) {
    return false;
  }
  if (override === "off") {
    return false;
  }
  if (override === "on") {
    return true;
  }
  return isRexProject;
}

// titleInsertion is the edit behind the title_missing quick fix (#58): where
// to insert (0-based line) and what. Inside an existing frontmatter block
// it's a line right after the opening '---'; with no block at all it's a
// whole new block at the top of the file.
export function titleInsertion(text: string): { line: number; text: string } {
  return /^---\r?\n/.test(text) ? { line: 1, text: 'title: ""\n' } : { line: 0, text: '---\ntitle: ""\n---\n\n' };
}

// LINT_ON_EDIT_DELAY_MS debounces linting unsaved buffers while typing.
export const LINT_ON_EDIT_DELAY_MS = 500;
