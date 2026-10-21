import { test } from "node:test";
import * as assert from "node:assert";
import {
  parseValidateOutput,
  groupIssuesByFile,
  effectiveLintEnabled,
  diagnosticLine,
  LINTED_ARTIFACTS_GLOB,
  Issue,
  titleInsertion,
} from "../src/lint";

test("parseValidateOutput decodes a valid issues envelope", () => {
  const result = parseValidateOutput(
    '{"issues":[{"file":"reports/vuln-1.md","check":"cvss_missing","message":"No CVSS data.","severity":"error"}],"errors":1,"warnings":0}'
  );
  assert.deepStrictEqual(result, {
    issues: [{ file: "reports/vuln-1.md", check: "cvss_missing", message: "No CVSS data.", severity: "error" }],
    errors: 1,
    warnings: 0,
  });
});

test("parseValidateOutput decodes an empty issues envelope", () => {
  const result = parseValidateOutput('{"issues":[],"errors":0,"warnings":0}');
  assert.deepStrictEqual(result, { issues: [], errors: 0, warnings: 0 });
});

test("parseValidateOutput decodes the fatal-error shape", () => {
  const result = parseValidateOutput('{"error":"index.md not found in ."}');
  assert.deepStrictEqual(result, { error: "index.md not found in ." });
});

test("parseValidateOutput returns null for malformed/empty output", () => {
  assert.strictEqual(parseValidateOutput("not json"), null);
  assert.strictEqual(parseValidateOutput(""), null);
  assert.strictEqual(parseValidateOutput("   \n  "), null);
  assert.strictEqual(parseValidateOutput('{"unexpected":true}'), null);
});

test("groupIssuesByFile buckets by file, preserving order", () => {
  const issues: Issue[] = [
    { file: "reports/vuln-1.md", check: "cvss_missing", message: "a", severity: "error" },
    { file: "reports/vuln-2.md", check: "timeline_missing", message: "b", severity: "error" },
    { file: "reports/vuln-1.md", check: "stub_marker_present", message: "c", severity: "error" },
  ];
  const grouped = groupIssuesByFile(issues);
  assert.deepStrictEqual(
    grouped.get("reports/vuln-1.md")?.map((i) => i.check),
    ["cvss_missing", "stub_marker_present"]
  );
  assert.deepStrictEqual(
    grouped.get("reports/vuln-2.md")?.map((i) => i.check),
    ["timeline_missing"]
  );
});

test("effectiveLintEnabled: lint.enabled is the master switch", () => {
  assert.strictEqual(effectiveLintEnabled(false, "on", true), false);
  assert.strictEqual(effectiveLintEnabled(false, "auto", true), false);
});

test("effectiveLintEnabled: projectOverride off/on short-circuit detection", () => {
  assert.strictEqual(effectiveLintEnabled(true, "off", true), false);
  assert.strictEqual(effectiveLintEnabled(true, "on", false), true);
});

test("effectiveLintEnabled: auto defers to detection", () => {
  assert.strictEqual(effectiveLintEnabled(true, "auto", true), true);
  assert.strictEqual(effectiveLintEnabled(true, "auto", false), false);
});

test("Issue line is optional and decodes from validate output", () => {
  const result = parseValidateOutput(
    '{"issues":[{"file":"artifacts/connections.json","check":"connections_invalid","message":"bad","severity":"error","line":4}],"errors":1,"warnings":0}'
  );
  assert.ok(result && "issues" in result);
  assert.strictEqual(result.issues[0].line, 4);
});

test("diagnosticLine: 1-based issue line to 0-based editor line, else top of file", () => {
  const base: Issue = { file: "a.md", check: "x", message: "m", severity: "warning" };
  assert.strictEqual(diagnosticLine({ ...base, line: 7 }), 6);
  assert.strictEqual(diagnosticLine({ ...base, line: 0 }), 0);
  assert.strictEqual(diagnosticLine(base), 0);
});

test("LINTED_ARTIFACTS_GLOB names the network/chart artifact files", () => {
  for (const name of ["assets", "connections", "webfindings"]) {
    assert.ok(LINTED_ARTIFACTS_GLOB.includes(name));
  }
  assert.ok(LINTED_ARTIFACTS_GLOB.startsWith("**/artifacts/"));
});

test("titleInsertion adds a line inside existing frontmatter", () => {
  assert.deepStrictEqual(titleInsertion("---\nseverity: High\n---\nBody\n"), { line: 1, text: 'title: ""\n' });
  assert.deepStrictEqual(titleInsertion("---\r\nseverity: High\r\n---\r\n"), { line: 1, text: 'title: ""\n' });
});

test("titleInsertion adds a whole block when there's no frontmatter", () => {
  assert.deepStrictEqual(titleInsertion("## Description\n"), { line: 0, text: '---\ntitle: ""\n---\n\n' });
});
