import * as assert from "assert";
import { ChildProcessWithoutNullStreams, spawn } from "child_process";
import * as fs from "fs";
import * as https from "https";
import * as path from "path";
import * as vscode from "vscode";
import type { RexApi } from "../../../src/extension";

// End-to-end: this extension in a real VS Code, a real rex-svr, and a
// second teammate ("bob") driven through the real rex CLI. runTest.ts sets
// the REX_IT_* environment.

const BIN = process.env.REX_IT_BIN!;
const PORT = Number(process.env.REX_IT_PORT);
const ALICE = process.env.REX_IT_ALICE!;
const BOB = process.env.REX_IT_BOB!;
const PASSWORD = process.env.REX_PASSWORD!;

const conn = (user: string) => ["--host", "127.0.0.1", "--port", String(PORT), "--username", user, "--insecure"];

async function waitFor(what: string, pred: () => boolean | Promise<boolean>, timeoutMs = 30_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      if (await pred()) {
        return;
      }
    } catch {
      // not yet
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`timed out after ${timeoutMs}ms waiting for: ${what}`);
}

const read = (dir: string, rel: string) => fs.readFileSync(path.join(dir, rel), "utf8");
const exists = (dir: string, rel: string) => fs.existsSync(path.join(dir, rel));
const write = (dir: string, rel: string, text: string) => {
  fs.mkdirSync(path.dirname(path.join(dir, rel)), { recursive: true });
  fs.writeFileSync(path.join(dir, rel), text);
};

function api(method: string, p: string, token?: string, body?: string): Promise<{ status: number; body: string }> {
  return new Promise((resolve, reject) => {
    const req = https.request(
      {
        host: "127.0.0.1",
        port: PORT,
        path: "/api/v1" + p,
        method,
        rejectUnauthorized: false,
        headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      },
      (res) => {
        let data = "";
        res.on("data", (d) => (data += d));
        res.on("end", () => resolve({ status: res.statusCode ?? 0, body: data }));
      }
    );
    req.on("error", reject);
    req.end(body);
  });
}

let token = "";
async function serverFile(rel: string): Promise<string | null> {
  if (!token) {
    token = JSON.parse((await api("POST", "/auth/login", undefined, JSON.stringify({ username: "probe", password: PASSWORD }))).body).token;
  }
  const r = await api("GET", "/files/" + rel, token);
  return r.status === 200 ? r.body : null;
}

// Bob: a plain `rex sync` in his own directory, like a teammate on the CLI.
class Bob {
  private proc: ChildProcessWithoutNullStreams | null = null;
  presence: ChildProcessWithoutNullStreams | null = null;
  startSync() {
    this.proc = spawn(BIN, ["sync", "--json", "--dir", BOB, ...conn("bob")], { env: { ...process.env, REX_PASSWORD: PASSWORD } });
    this.proc.stdout.resume();
    this.proc.stderr.resume();
  }
  stopSync() {
    this.proc?.stdin.end();
    this.proc?.kill();
    this.proc = null;
  }
  startPresence() {
    this.presence = spawn(BIN, ["presence", ...conn("bob")], { env: { ...process.env, REX_PASSWORD: PASSWORD } });
    this.presence.stdout.resume();
    this.presence.stderr.resume();
  }
  dispose() {
    this.stopSync();
    this.presence?.kill();
  }
}

suite("Report Rex extension against a live rex-svr", () => {
  let rex: RexApi;
  const bob = new Bob();

  suiteSetup(async () => {
    const ext = vscode.extensions.getExtension<RexApi>("report-rex.rex-presence");
    assert.ok(ext, "extension not found");
    rex = await ext.activate();
    await waitFor("config to load", () => rex.controller.cfg !== null);
  });

  suiteTeardown(() => {
    bob.dispose();
    rex?.controller.stopSync(false);
  });

  test("activates configured, with sync stopped by default and presence online", async () => {
    const c = rex.controller;
    assert.strictEqual(c.sync.phase, "stopped");
    assert.strictEqual(c.syncRunning, false);
    await waitFor("presence bridge to connect", () => c.presenceConnected === true);
    const rows = await rex.views.sync();
    assert.ok(rows.some((n) => n.item.label === "Sync: Stopped"), "Sync row should say Stopped");
  });

  test("Test Connection reports a healthy, authenticated server", async () => {
    const rep = await rex.controller.testConnection(false);
    assert.ok(rep && !rep.error, rep?.error);
    assert.strictEqual(rep!.health?.status, "ok");
    assert.ok(rep!.authenticated);
  });

  test("Start Sync connects and pushes the workspace to a teammate (but never .vscode/)", async () => {
    const started = await vscode.commands.executeCommand<boolean>("report-rex.startSync");
    assert.strictEqual(started, true);
    await waitFor("sync to connect", () => rex.controller.sync.phase === "connected");
    bob.startSync();
    await waitFor("bob to receive the files", () => exists(BOB, "index.md") && exists(BOB, "sections/notes.md"));
    assert.ok(!exists(BOB, ".vscode/settings.json"), "per-user editor settings must not sync");
  });

  test("a teammate deletes index.md and it's re-created: no conflict (issue #42)", async () => {
    fs.rmSync(path.join(BOB, "index.md"));
    await waitFor("alice to see the delete", () => !exists(ALICE, "index.md"));
    write(ALICE, "index.md", "---\ntitle: Integration\n---\n\n# Re-created\n");
    await waitFor("bob to get the re-created file", () => exists(BOB, "index.md") && read(BOB, "index.md").includes("Re-created"));
    await rex.controller.refresh();
    assert.strictEqual(rex.controller.conflicts.length, 0, JSON.stringify(rex.controller.conflicts));
  });

  test("edits to different lines of one file merge automatically", async () => {
    rex.controller.stopSync();
    await waitFor("sync to stop", () => rex.controller.sync.phase === "stopped");
    write(ALICE, "sections/notes.md", "ONE (alice)\ntwo\nthree\nfour\nfive\n");
    write(BOB, "sections/notes.md", "one\ntwo\nthree\nfour\nFIVE (bob)\n");
    await waitFor("bob's edit to reach the server", async () => (await serverFile("sections/notes.md"))?.includes("FIVE (bob)") ?? false);
    assert.ok(await rex.controller.startSync());
    const merged = "ONE (alice)\ntwo\nthree\nfour\nFIVE (bob)\n";
    await waitFor("alice's copy to be merged", () => read(ALICE, "sections/notes.md") === merged);
    await waitFor("bob to get the merge", () => read(BOB, "sections/notes.md") === merged);
    await rex.controller.refresh();
    assert.strictEqual(rex.controller.conflicts.length, 0);
    assert.ok(rex.controller.sync.merged >= 1);
  });

  test("Recent Changes shows the teammate's edit", async () => {
    await rex.controller.refresh();
    await waitFor("bob in recent changes", () => rex.controller.recent.some((e) => e.modifiedBy === "bob"));
    assert.ok(rex.views.changes().some((n) => String(n.item.description).includes("bob")));
  });

  test("overlapping edits become one conflict of yours; Take Server resolves it", async () => {
    rex.controller.stopSync();
    await waitFor("sync to stop", () => rex.controller.sync.phase === "stopped");
    write(ALICE, "sections/notes.md", "ONE (alice again)\ntwo\nthree\nfour\nFIVE (bob)\n");
    write(BOB, "sections/notes.md", "ONE (bob wins)\ntwo\nthree\nfour\nFIVE (bob)\n");
    await waitFor("bob's edit to reach the server", async () => (await serverFile("sections/notes.md"))?.includes("bob wins") ?? false);
    assert.ok(await rex.controller.startSync());
    await waitFor("the conflict to show up", async () => {
      await rex.controller.refresh();
      return rex.controller.myConflicts().length === 1;
    });
    const groups = rex.views.conflicts();
    assert.strictEqual(groups[0].item.label, "Yours");
    assert.strictEqual(groups[0].children?.[0].conflict?.path, "sections/notes.md");
    // Keep typing while conflicted: still exactly one open conflict.
    write(ALICE, "sections/notes.md", "ONE (alice, still typing)\ntwo\nthree\nfour\nFIVE (bob)\n");
    rex.controller.syncNow();
    await new Promise((r) => setTimeout(r, 3000));
    await rex.controller.refresh();
    assert.strictEqual(rex.controller.myConflicts().length, 1);

    assert.ok(await rex.controller.resolve(rex.controller.myConflicts()[0], "remote"));
    assert.strictEqual(read(ALICE, "sections/notes.md"), "ONE (bob wins)\ntwo\nthree\nfour\nFIVE (bob)\n");
    await rex.controller.refresh();
    assert.strictEqual(rex.controller.conflicts.length, 0);
    // And alice is back in sync: her next edit goes straight through.
    write(ALICE, "sections/notes.md", "ONE (bob wins)\ntwo\nthree\nfour\nFIVE (bob)\nsix (alice)\n");
    await waitFor("bob to get alice's follow-up", () => read(BOB, "sections/notes.md").includes("six (alice)"));
  });

  test("Team shows a teammate who has a file open", async () => {
    bob.startPresence();
    bob.presence!.stdin.write(JSON.stringify({ op: "update", file: "index.md", state: "editing", dirty: true }) + "\n");
    await waitFor("bob in the Team view", () => rex.views.team().some((n) => n.item.label === "bob"));
    const bobNode = rex.views.team().find((n) => n.item.label === "bob")!;
    assert.strictEqual(bobNode.children?.[0].item.label, "index.md");
  });

  test("Stop Sync ends the background process", async () => {
    rex.controller.stopSync();
    await waitFor("sync to stop", () => rex.controller.sync.phase === "stopped" && !rex.controller.syncRunning);
    const flag = vscode.workspace.getConfiguration("report-rex").get<boolean>("sync.enabled");
    assert.strictEqual(flag, false);
    write(ALICE, "offline-only.md", "not synced\n");
    await new Promise((r) => setTimeout(r, 3000));
    assert.ok(!exists(BOB, "offline-only.md"), "a stopped sync must not upload");
  });
});
