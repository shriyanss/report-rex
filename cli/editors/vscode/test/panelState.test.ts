import { test } from "node:test";
import * as assert from "node:assert";
import { panelRows, PanelState, configured } from "../src/panelState";
import { initialSyncState } from "../src/syncState";

function state(overrides: Partial<PanelState>): PanelState {
  return {
    sync: initialSyncState(),
    syncEnabled: false,
    presenceConnected: undefined,
    myConflicts: 0,
    otherConflicts: 0,
    host: "sync.example",
    port: 9443,
    username: "alice",
    dir: "/r",
    passwordSet: true,
    caCert: "",
    insecure: false,
    lintEnabled: true,
    ...overrides,
  };
}

const row = (s: PanelState, label: string) => panelRows(s).find((r) => r.label === label || r.label.startsWith(label))!;

test("sync row: stopped offers Start, running offers Stop", () => {
  const stopped = row(state({}), "Sync:");
  assert.strictEqual(stopped.label, "Sync: Stopped");
  assert.strictEqual(stopped.command, "report-rex.startSync");
  const running = row(state({ syncEnabled: true, sync: { ...initialSyncState(), phase: "connected" } }), "Sync:");
  assert.strictEqual(running.label, "Sync: Connected");
  assert.strictEqual(running.command, "report-rex.stopSync");
});

test("sync row surfaces the last error while not connected", () => {
  const r = row(state({ syncEnabled: true, sync: { ...initialSyncState(), phase: "error", lastError: "login: 401" } }), "Sync:");
  assert.strictEqual(r.description, "login: 401");
});

test("server row: live presence without a test, then test results", () => {
  assert.match(row(state({}), "Server").description!, /Unknown/);
  assert.strictEqual(row(state({ presenceConnected: true }), "Server").description, "Online");
  assert.strictEqual(row(state({ presenceConnected: false }), "Server").description, "Offline");
  const ok = row(
    state({
      health: {
        server: "s:1", user: "alice", reachable: true, latency_ms: 12, authenticated: true, files: 3,
        recent: [], conflicts: [], health: { status: "ok", git_ok: true, db_ok: true, pending_materializations: 0, version: "0.7.0" },
      },
    }),
    "Server"
  );
  assert.strictEqual(ok.description, "Online · rex-svr 0.7.0 · 12 ms");
  const bad = row(
    state({ health: { server: "", user: "", reachable: false, latency_ms: 0, authenticated: false, files: 0, recent: [], conflicts: [], error: "x509", hint: "tls" } }),
    "Server"
  );
  assert.strictEqual(bad.description, "Failed (tls)");
  assert.strictEqual(bad.command, "report-rex.testConnection");
});

test("TLS row flags the missing trust setting", () => {
  assert.match(row(state({}), "TLS").description!, /required/);
  assert.match(row(state({ caCert: "/c.crt" }), "TLS").description!, /Pinned/);
  assert.match(row(state({ insecure: true, caCert: "/c.crt" }), "TLS").description!, /Insecure/);
});

test("conflicts row counts yours and teammates'", () => {
  assert.strictEqual(row(state({}), "Conflicts").description, "None");
  const r = row(state({ myConflicts: 2, otherConflicts: 1 }), "Conflicts");
  assert.strictEqual(r.description, "2 yours, 1 teammates'");
  assert.strictEqual(r.icon, "warning");
});

test("configured needs host and username", () => {
  assert.ok(configured({ host: "h", username: "u" }));
  assert.ok(!configured({ host: "h", username: "" }));
});
