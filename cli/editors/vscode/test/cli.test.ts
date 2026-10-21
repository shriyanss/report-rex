import { test } from "node:test";
import * as assert from "node:assert";
import { blameAnnotation, blameArgs, compareVersions, connArgs, formatBlame, parseStatus, parseTemplateList, rexEnv, rexVersionOk, timeAgo } from "../src/cli";

const cfg = { host: "h", port: 9443, username: "alice", insecure: false, caCert: "/c.crt", password: "pw", bin: "rex" };

test("connArgs carries host/port/user and TLS trust, never the password", () => {
  const a = connArgs(cfg);
  assert.deepStrictEqual(a, ["--host", "h", "--port", "9443", "--username", "alice", "--ca-cert", "/c.crt"]);
  assert.ok(!a.includes("pw"));
  assert.deepStrictEqual(connArgs({ ...cfg, insecure: true }).slice(-1), ["--insecure"]);
  assert.strictEqual(rexEnv(cfg).REX_PASSWORD, "pw");
});

test("parseStatus: real report, failed report, and non-JSON output", () => {
  const ok = parseStatus({ code: 0, stdout: '{"reachable":true,"authenticated":true,"files":2,"recent":[],"conflicts":[]}', stderr: "" });
  assert.ok(ok.reachable && !ok.error);
  const bad = parseStatus({ code: 1, stdout: '{"reachable":false,"error":"x509","hint":"tls","recent":[],"conflicts":[]}', stderr: "" });
  assert.strictEqual(bad.hint, "tls");
  const old = parseStatus({ code: 1, stdout: "", stderr: 'Error: unknown command "status" for "rex"' });
  assert.match(old.error!, /unknown command/);
  const missing = parseStatus({ code: null, stdout: "", stderr: "", error: "spawn rex ENOENT" });
  assert.strictEqual(missing.hint, "config");
});

test("version checks", () => {
  assert.ok(compareVersions("0.7.0", "0.6.2") > 0);
  assert.ok(compareVersions("v0.10.0", "0.9.9") > 0);
  assert.strictEqual(compareVersions("0.7.0-dev", "0.7.0"), 0);
  assert.ok(rexVersionOk("0.7.0"));
  assert.ok(!rexVersionOk("0.6.2"));
  assert.ok(rexVersionOk("dev"));
});

test("parseTemplateList: name<TAB>title lines, title falls back to name", () => {
  assert.deepStrictEqual(parseTemplateList("csrf\tCSRF in [ACTION]\nssrf\n\n"), [
    { name: "csrf", title: "CSRF in [ACTION]" },
    { name: "ssrf", title: "ssrf" },
  ]);
});

test("blame: args, relative time, annotation and tab text", () => {
  assert.deepStrictEqual(blameArgs("reports/a.md"), ["blame", "--json", "reports/a.md"]);
  const now = Date.parse("2026-10-01T12:00:00Z");
  assert.strictEqual(timeAgo("2026-10-01T11:55:00Z", now), "5 minutes ago");
  assert.strictEqual(timeAgo("2026-09-30T12:00:00Z", now), "1 day ago");
  assert.strictEqual(timeAgo("2026-10-01T12:00:00Z", now), "just now");
  const l = { line: 1, commit: "abcdef1234", user: "alice", time: "2026-10-01T10:00:00Z", summary: "sync: 1 file(s) by alice", text: "hi" };
  assert.strictEqual(blameAnnotation(l, now), "alice, 2 hours ago • sync: 1 file(s) by alice");
  assert.strictEqual(formatBlame({ path: "a", lines: [l] }), "abcdef12 alice 2026-10-01    1| hi");
});
