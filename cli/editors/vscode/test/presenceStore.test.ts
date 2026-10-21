import { test } from "node:test";
import * as assert from "node:assert";
import {
  createRemoteState,
  applyPresenceEvent,
  othersOn,
  isConcurrentEdit,
  statusText,
  isEditing,
} from "../src/presenceStore";
import { PresenceEvent } from "../src/presenceBridge";

function ev(overrides: Partial<PresenceEvent>): PresenceEvent {
  return {
    type: "presence",
    username: "alice",
    session_id: "s1",
    file: "reports/vuln-1.md",
    state: "viewing",
    dirty: false,
    action: "active",
    ...overrides,
  };
}

test("applyPresenceEvent sets and clears entries", () => {
  const remote = createRemoteState();
  applyPresenceEvent(remote, ev({}));
  assert.deepStrictEqual(othersOn(remote, "reports/vuln-1.md", "bob"), [
    { username: "alice", state: "viewing", dirty: false },
  ]);

  applyPresenceEvent(remote, ev({ action: "idle" }));
  assert.deepStrictEqual(othersOn(remote, "reports/vuln-1.md", "bob"), []);
});

test("othersOn excludes the caller's own username", () => {
  const remote = createRemoteState();
  applyPresenceEvent(remote, ev({ username: "bob" }));
  assert.deepStrictEqual(othersOn(remote, "reports/vuln-1.md", "bob"), []);
});

test("isConcurrentEdit only fires when someone else is editing", () => {
  const remote = createRemoteState();
  applyPresenceEvent(remote, ev({ state: "viewing" }));
  assert.strictEqual(isConcurrentEdit(remote, "reports/vuln-1.md", "bob"), null);

  applyPresenceEvent(remote, ev({ state: "editing" }));
  assert.strictEqual(isConcurrentEdit(remote, "reports/vuln-1.md", "bob"), "alice");
});

test("statusText prefers editing over viewing, and is empty when alone", () => {
  const remote = createRemoteState();
  assert.strictEqual(statusText(remote, "reports/vuln-1.md", "bob"), "");

  applyPresenceEvent(remote, ev({ session_id: "s1", username: "alice", state: "viewing" }));
  assert.strictEqual(statusText(remote, "reports/vuln-1.md", "bob"), "⚠ alice viewing");

  applyPresenceEvent(remote, ev({ session_id: "s2", username: "carol", state: "editing" }));
  assert.strictEqual(statusText(remote, "reports/vuln-1.md", "bob"), "⚠ carol editing");
});

test("isEditing drives the decoration badge", () => {
  const remote = createRemoteState();
  assert.strictEqual(isEditing(remote, "reports/vuln-1.md", "bob"), null);

  applyPresenceEvent(remote, ev({ state: "editing" }));
  assert.deepStrictEqual(isEditing(remote, "reports/vuln-1.md", "bob"), {
    username: "alice",
    state: "editing",
    dirty: false,
  });
});
