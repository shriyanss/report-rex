import { spawn } from "child_process";
import { parseValidateOutput, ValidateResult, ValidateError } from "./lint";

// runLint spawns `<bin> report validate --dir <dir> --json` once and parses
// its stdout. One-shot, not a long-lived NDJSON stream like
// presenceBridge.ts/syncBridge.ts -- `report validate` exits after a single
// run, so there's no process lifecycle to manage between lint passes.
// Resolves to null (never throws) on spawn failure -- e.g. the configured
// bin isn't on $PATH -- linting must never block editing.
// overlay ({absolute path: content}) lints unsaved buffers in place of the
// files on disk, via `--stdin`.
export function runLint(
  dir: string,
  bin: string,
  overlay?: Record<string, string>
): Promise<ValidateResult | ValidateError | null> {
  return new Promise((resolve) => {
    let proc;
    try {
      proc = spawn(bin, ["report", "validate", "--dir", dir, "--json", ...(overlay ? ["--stdin"] : [])]);
    } catch {
      resolve(null);
      return;
    }
    proc.stdin.on("error", () => undefined); // rex exited before reading it
    proc.stdin.end(overlay ? JSON.stringify(overlay) : undefined);
    let stdout = "";
    proc.stdout.on("data", (chunk) => (stdout += chunk));
    proc.on("error", () => resolve(null));
    proc.on("close", () => resolve(parseValidateOutput(stdout)));
  });
}
