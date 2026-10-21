import { test } from "node:test";
import * as assert from "node:assert";
import { backoffMs, initialSyncState, nextSyncState, parseSyncLine, SyncLine } from "../src/syncState";

const fold = (lines: Partial<SyncLine>[]) =>
  lines.reduce((s, l) => nextSyncState(s, { time: "2026-09-27T10:00:00Z", ...l } as SyncLine), initialSyncState());

test("parseSyncLine accepts rex sync --json lines and rejects junk", () => {
  assert.deepStrictEqual(parseSyncLine('{"type":"ready","time":"t","user":"alice"}'), { type: "ready", time: "t", user: "alice" });
  assert.strictEqual(parseSyncLine(""), null);
  assert.strictEqual(parseSyncLine("Report Rex Sync"), null);
  assert.strictEqual(parseSyncLine('{"no":"type"}'), null);
});

test("lifecycle: ready -> connected -> offline -> reconnecting -> connected -> stopped", () => {
  assert.strictEqual(fold([{ type: "ready" }]).phase, "connected");
  const off = fold([{ type: "ready" }, { type: "status", state: "offline", message: "EOF" }]);
  assert.strictEqual(off.phase, "offline");
  assert.strictEqual(off.lastError, "EOF");
  const back = fold([{ type: "ready" }, { type: "status", state: "offline", message: "EOF" }, { type: "status", state: "reconnecting" }, { type: "status", state: "connected" }]);
  assert.strictEqual(back.phase, "connected");
  assert.strictEqual(back.lastError, undefined);
  assert.strictEqual(fold([{ type: "ready" }, { type: "status", state: "stopped" }]).phase, "stopped");
});

test("events are counted; conflicts don't count as a sync", () => {
  const s = fold([
    { type: "ready" },
    { type: "event", kind: "pushed", path: "a.md", time: "T1" },
    { type: "event", kind: "pulled", path: "b.md", time: "T2" },
    { type: "event", kind: "merged", path: "c.md", time: "T3" },
    { type: "event", kind: "conflict", path: "d.md", time: "T4", conflict_id: 7 },
  ]);
  assert.deepStrictEqual([s.pushed, s.pulled, s.merged, s.conflicts], [1, 1, 1, 1]);
  assert.strictEqual(s.lastSyncAt, "T3");
});

test("reconcile errors are remembered without changing the phase", () => {
  const s = fold([{ type: "ready" }, { type: "error", scope: "reconcile", message: "boom" }]);
  assert.strictEqual(s.phase, "connected");
  assert.strictEqual(s.lastError, "boom");
});

test("backoff doubles from 1s and caps at 30s", () => {
  assert.deepStrictEqual([0, 1, 2, 3, 4, 5, 9].map(backoffMs), [1000, 2000, 4000, 8000, 16000, 30000, 30000]);
});
