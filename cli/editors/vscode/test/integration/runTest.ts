// Launches a real VS Code (downloaded by @vscode/test-electron) against a
// real rex-svr, with this extension loaded from source, and runs
// suite/*.test.js inside it. Needs a display: run it under xvfb-run on a
// headless box (scripts/test-headless.sh does all of this in Docker).
//
//   REX_BIN_DIR=<dir with rex + rex-svr> npm run test:integration
import { ChildProcess, spawn } from "child_process";
import * as fs from "fs";
import * as https from "https";
import * as net from "net";
import * as os from "os";
import * as path from "path";
import { runTests } from "@vscode/test-electron";

const PASSWORD = "it-pass";

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.listen(0, "127.0.0.1", () => {
      const port = (srv.address() as net.AddressInfo).port;
      srv.close(() => resolve(port));
    });
    srv.on("error", reject);
  });
}

function healthy(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const req = https.get({ host: "127.0.0.1", port, path: "/api/v1/healthz", rejectUnauthorized: false, timeout: 1000 }, (res) => {
      res.resume();
      resolve(res.statusCode === 200);
    });
    req.on("error", () => resolve(false));
    req.on("timeout", () => req.destroy());
  });
}

async function main(): Promise<void> {
  const binDir = process.env.REX_BIN_DIR;
  if (!binDir) {
    throw new Error("set REX_BIN_DIR to a directory containing rex and rex-svr");
  }
  const rex = path.resolve(binDir, "rex");
  const svr = path.resolve(binDir, "rex-svr");
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "rex-vscode-it-"));
  const serverDir = path.join(root, "server");
  const alice = path.join(root, "alice");
  const bob = path.join(root, "bob");
  for (const d of [serverDir, alice, bob, path.join(alice, ".vscode"), path.join(alice, "sections")]) {
    fs.mkdirSync(d, { recursive: true });
  }
  fs.writeFileSync(path.join(serverDir, "pw"), PASSWORD);
  const port = await freePort();

  const log = fs.openSync(path.join(root, "rex-svr.log"), "w");
  const server: ChildProcess = spawn(
    svr,
    ["--port", String(port), "--password-file", path.join(serverDir, "pw"), "--db", path.join(serverDir, "db"),
     "--workspace", path.join(serverDir, "git"), "--cert", path.join(serverDir, "c.crt"), "--key", path.join(serverDir, "c.key")],
    { stdio: ["ignore", log, log] }
  );
  try {
    for (let i = 0; i < 50 && !(await healthy(port)); i++) {
      await new Promise((r) => setTimeout(r, 200));
    }
    if (!(await healthy(port))) {
      throw new Error(`rex-svr never became healthy; see ${root}/rex-svr.log`);
    }

    fs.writeFileSync(path.join(alice, "index.md"), "---\ntitle: Integration\n---\n\n# Integration report\n");
    fs.writeFileSync(path.join(alice, "sections", "notes.md"), "one\ntwo\nthree\nfour\nfive\n");
    fs.writeFileSync(
      path.join(alice, ".vscode", "settings.json"),
      JSON.stringify(
        {
          "report-rex.host": "127.0.0.1",
          "report-rex.port": port,
          "report-rex.username": "alice",
          "report-rex.insecure": true,
          "report-rex.bin": rex,
          "report-rex.lint.enabled": false,
        },
        null,
        2
      )
    );

    const ext = path.resolve(__dirname, "../../..");
    await runTests({
      version: process.env.VSCODE_VERSION || "stable",
      extensionDevelopmentPath: ext,
      extensionTestsPath: path.join(__dirname, "suite", "index"),
      launchArgs: [
        alice,
        "--disable-extensions",
        "--disable-workspace-trust",
        "--skip-welcome",
        "--skip-release-notes",
        "--no-sandbox", // containers run as root
        "--disable-gpu",
        "--user-data-dir",
        path.join(root, "user-data"),
      ],
      extensionTestsEnv: {
        REX_PASSWORD: PASSWORD,
        REX_IT_BIN: rex,
        REX_IT_PORT: String(port),
        REX_IT_ALICE: alice,
        REX_IT_BOB: bob,
      },
    });
  } finally {
    server.kill();
    if (!process.env.REX_IT_KEEP) {
      fs.rmSync(root, { recursive: true, force: true });
    } else {
      console.log(`kept ${root}`);
    }
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
