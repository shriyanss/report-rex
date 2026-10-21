import { test } from "node:test";
import * as assert from "node:assert";
import * as path from "path";
import {
  hasConflictMarkers, mergeRecent, parseHandoff, relativeTime, relativeTo, splitConflicts,
  statusBarText, teamFromPresence, RecentEntry,
} from "../src/model";
import { createRemoteState, applyPresenceEvent } from "../src/presenceStore";
import { initialSyncState } from "../src/syncState";

const c = (id: number, user: string, created: string) => ({ id, path: `f${id}.md`, kind: "divergent_edit", remote_revision: 1, local_username: user, created_at: created });

test("splitConflicts: yours vs teammates', newest first", () => {
  const g = splitConflicts([c(1, "alice", "2026-01-01"), c(2, "bob", "2026-01-02"), c(3, "alice", "2026-01-03")], "alice");
  assert.deepStrictEqual(g.mine.map((x) => x.id), [3, 1]);
  assert.deepStrictEqual(g.others.map((x) => x.id), [2]);
});

test("mergeRecent: newest on top, one row per path, capped", () => {
  const e = (p: string, at: string): RecentEntry => ({ path: p, revision: 1, modifiedBy: "bob", at, deleted: false });
  let list = [e("a", "1"), e("b", "2")];
  list = mergeRecent(list, e("a", "3"));
  assert.deepStrictEqual(list.map((x) => x.path + x.at), ["a3", "b2"]);
  assert.strictEqual(mergeRecent(list, e("c", "4"), 2).length, 2);
});

test("relativeTime buckets", () => {
  const now = Date.parse("2026-09-27T12:00:00Z");
  assert.strictEqual(relativeTime("2026-09-27T11:59:58Z", now), "just now");
  assert.strictEqual(relativeTime("2026-09-27T11:59:00Z", now), "1m ago");
  assert.strictEqual(relativeTime("2026-09-27T09:00:00Z", now), "3h ago");
  assert.strictEqual(relativeTime("2026-09-25T12:00:00Z", now), "2d ago");
  assert.strictEqual(relativeTime(undefined, now), "never");
});

test("teamFromPresence groups teammates' files, editing first, excluding you", () => {
  const r = createRemoteState();
  const ev = (username: string, session_id: string, file: string, state: string) =>
    applyPresenceEvent(r, { type: "presence", username, session_id, file, state, dirty: false, action: "active" });
  ev("bob", "s1", "b.md", "viewing");
  ev("bob", "s1", "a.md", "editing");
  ev("alice", "s2", "a.md", "editing");
  ev("carol", "s3", "c.md", "viewing");
  const team = teamFromPresence(r, "alice");
  assert.deepStrictEqual(team.map((m) => m.username), ["bob", "carol"]);
  assert.deepStrictEqual(team[0].files.map((f) => f.file), ["a.md", "b.md"]);
});

test("parseHandoff reads frontmatter and the title heading", () => {
  const h = parseHandoff(
    "handoffs/alice/investigate.md",
    '---\nrex_handoff_id: 8f\nfrom: bob\nto: alice\nstatus: open\ncreated_at: "2026"\n---\n\n# Investigate the API\n\nbody\n'
  );
  assert.deepStrictEqual([h.title, h.from, h.to, h.status], ["Investigate the API", "bob", "alice", "open"]);
  assert.strictEqual(parseHandoff("handoffs/alice/x.md", "no frontmatter").title, "x");
});

test("relativeTo is relative to the synced dir, null outside it", () => {
  const root = path.resolve("/tmp/rep");
  assert.strictEqual(relativeTo(root, path.join(root, "sections", "a.md")), "sections/a.md");
  assert.strictEqual(relativeTo(root, path.resolve("/tmp/other/a.md")), null);
  assert.strictEqual(relativeTo(root, root), null);
  assert.strictEqual(relativeTo("", "/x"), null);
});

test("hasConflictMarkers matches rex's markers only when complete", () => {
  assert.ok(hasConflictMarkers("<<<<<<< LOCAL\na\n=======\nb\n>>>>>>> REMOTE\n"));
  assert.ok(!hasConflictMarkers("<<<<<<< LOCAL\na\n"));
});

test("statusBarText shows state, your conflicts, and presence", () => {
  assert.strictEqual(statusBarText(initialSyncState(), 0, ""), "$(circle-slash) Rex (sync off)");
  assert.strictEqual(statusBarText({ ...initialSyncState(), phase: "connected" }, 2, "⚠ bob editing"), "$(cloud) Rex $(warning) 2 · ⚠ bob editing");
});
