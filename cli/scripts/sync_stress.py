#!/usr/bin/env python3
"""Extreme-load stress test for rex-svr + `rex sync` (issue #42).

Spins up one rex-svr and N `rex sync` clients in a scratch directory, then:

  1. seeds a small report workspace and waits for every client to converge;
  2. deterministic delete -> re-create of index.md on two different clients
     (must not produce a conflict);
  3. chaos: every client mutates files every --interval seconds for
     --duration seconds (own file, own line of a shared file, whole-file
     gibberish on a hot file, delete/re-create index.md);
  4. quiesce and check: processes alive, <=1 open conflict per (path, user),
     no lost writes on single-writer files, git materialization drained;
  5. resolve every open conflict with `rex resolve --remote --id` and assert
     all clients converge byte-for-byte on the server.

Results are read straight from the server's SQLite DB and HTTP API, not from
client stdout, so the same script measures old and new binaries alike.

    python3 cli/scripts/sync_stress.py --rex ./rex --rex-svr ./rex-svr --duration 120

Stdlib only. Exits non-zero if any check fails.
"""

import argparse
import hashlib
import json
import os
import random
import shutil
import socket
import sqlite3
import ssl
import string
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request

PASSWORD = "stress-pass"
SSL_CTX = ssl._create_unverified_context()
EXCLUDED_DIRS = {".report-rex", ".git", ".vscode"}


def log(msg):
    print(f"[{time.strftime('%H:%M:%S')}] {msg}", flush=True)


def gibberish(n=40):
    return " ".join(
        "".join(random.choices(string.ascii_lowercase, k=random.randint(2, 9)))
        for _ in range(n)
    )


class Server:
    def __init__(self, bin_path, root):
        self.root = os.path.join(root, "server")
        os.makedirs(self.root)
        self.port = free_port()
        self.db = os.path.join(self.root, "rex-svr.db")
        pw = os.path.join(self.root, "password.txt")
        with open(pw, "w") as f:
            f.write(PASSWORD)
        self.log = open(os.path.join(self.root, "server.log"), "w")
        self.proc = subprocess.Popen(
            [bin_path, "--port", str(self.port), "--password-file", pw,
             "--db", self.db, "--workspace", os.path.join(self.root, "git"),
             "--cert", os.path.join(self.root, "c.crt"), "--key", os.path.join(self.root, "c.key")],
            stdout=self.log, stderr=subprocess.STDOUT, cwd=self.root)
        self.base = f"https://127.0.0.1:{self.port}/api/v1"
        wait_for(lambda: self.health() is not None, 30, "rex-svr healthz")
        self.token = self.post("/auth/login", {"username": "stress-admin", "password": PASSWORD})["token"]

    def req(self, path, method="GET", body=None, raw=False):
        headers = {"Content-Type": "application/json"}
        if getattr(self, "token", None):
            headers["Authorization"] = f"Bearer {self.token}"
        data = json.dumps(body).encode() if body is not None else None
        r = urllib.request.Request(self.base + path, data=data, method=method, headers=headers)
        with urllib.request.urlopen(r, context=SSL_CTX, timeout=10) as resp:
            out = resp.read()
        return out if raw else json.loads(out)

    def post(self, path, body):
        return self.req(path, "POST", body)

    def health(self):
        try:
            return self.req("/healthz")
        except Exception:
            return None

    def manifest(self):
        """{path: sha256} for every live (non-tombstoned) file."""
        return {f["path"]: f["sha256"] for f in self.req("/manifest")["files"] if not f["deleted"]}

    def content(self, path):
        try:
            return self.req("/files/" + path, raw=True)
        except Exception:
            return None

    def sql(self, query, args=()):
        for _ in range(20):
            try:
                con = sqlite3.connect(f"file:{self.db}?mode=ro", uri=True, timeout=5)
                try:
                    return con.execute(query, args).fetchall()
                finally:
                    con.close()
            except sqlite3.OperationalError:
                time.sleep(0.2)
        raise RuntimeError("sqlite stayed locked")


class Client:
    def __init__(self, idx, rex, root, port):
        self.idx, self.rex, self.port = idx, rex, port
        self.user = f"user{idx}"
        self.dir = os.path.join(root, f"client-{idx}")
        os.makedirs(self.dir)
        self.logf = open(os.path.join(root, f"client-{idx}.log"), "w")
        self.proc = None
        self.last_note = None

    def conn_args(self):
        return ["--host", "127.0.0.1", "--port", str(self.port), "--username", self.user,
                "--dir", self.dir, "--insecure"]

    def env(self):
        return {**os.environ, "REX_PASSWORD": PASSWORD}

    def start(self):
        self.proc = subprocess.Popen([self.rex, "sync", *self.conn_args()], stdout=self.logf,
                                     stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL, env=self.env())

    def run(self, *args):
        return subprocess.run([self.rex, *args, *self.conn_args()], capture_output=True, text=True,
                              env=self.env(), timeout=60)

    def path(self, rel):
        return os.path.join(self.dir, rel)

    def write(self, rel, text):
        p = self.path(rel)
        os.makedirs(os.path.dirname(p), exist_ok=True)
        with open(p, "w") as f:
            f.write(text)

    def files(self):
        out = {}
        for root, dirs, names in os.walk(self.dir):
            dirs[:] = [d for d in dirs if d not in EXCLUDED_DIRS]
            for n in names:
                if n == ".DS_Store" or n.endswith((".swp", ".swo", ".tmp")):
                    continue
                full = os.path.join(root, n)
                rel = os.path.relpath(full, self.dir).replace(os.sep, "/")
                try:
                    with open(full, "rb") as f:
                        out[rel] = hashlib.sha256(f.read()).hexdigest()
                except FileNotFoundError:
                    pass
        return out


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def wait_for(pred, timeout, what):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            if pred():
                return True
        except Exception:
            pass
        time.sleep(0.5)
    log(f"timed out after {timeout}s waiting for {what}")
    return False


def diverged(server, clients):
    """Clients whose workspace differs from the server's live manifest."""
    want = server.manifest()
    return [c.idx for c in clients if c.files() != want]


def conflict_count(server, path=None):
    q = "SELECT count(*) FROM conflicts" + (" WHERE path = ?" if path else "")
    return server.sql(q, (path,) if path else ())[0][0]


def chaos(clients, interval, duration):
    stop = threading.Event()
    ops = {"note": 0, "line": 0, "hot": 0, "index": 0, "errors": 0}
    lock = threading.Lock()

    def worker(c):
        while not stop.is_set():
            r = random.random()
            try:
                if r < 0.50:
                    c.last_note = f"# client {c.idx}\n\n{gibberish()}\n"
                    c.write(f"notes/client-{c.idx}.md", c.last_note)
                    op = "note"
                elif r < 0.80:
                    p = c.path("sections/shared.md")
                    with open(p) as f:
                        lines = f.read().split("\n")
                    if len(lines) > c.idx and not any(l.startswith("<<<<<<<") for l in lines):
                        lines[c.idx] = f"line {c.idx}: {gibberish(5)}"
                        c.write("sections/shared.md", "\n".join(lines))
                    op = "line"
                elif r < 0.95:
                    c.write("sections/hot.md", f"# hot\n\n{gibberish(80)}\n")
                    op = "hot"
                else:
                    p = c.path("index.md")
                    if os.path.exists(p):
                        os.remove(p)
                    else:
                        c.write("index.md", f"---\ntitle: stress\n---\n\n{gibberish(10)}\n")
                    op = "index"
            except (FileNotFoundError, IsADirectoryError):
                op = "errors"
            with lock:
                ops[op] += 1
            stop.wait(interval)

    threads = [threading.Thread(target=worker, args=(c,), daemon=True) for c in clients]
    for t in threads:
        t.start()
    stop.wait(duration)
    stop.set()
    for t in threads:
        t.join()
    return ops


def quiesce(server, min_wait=35, stable_polls=3, timeout=180):
    """Wait until the server manifest + open conflicts stop changing."""
    start, last, stable = time.time(), None, 0
    while time.time() - start < timeout:
        time.sleep(5)
        snap = (server.manifest(), server.sql("SELECT count(*), max(id) FROM conflicts WHERE status='open'"))
        stable = stable + 1 if snap == last else 0
        last = snap
        if stable >= stable_polls and time.time() - start >= min_wait:
            return time.time() - start
    return None


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--rex", required=True)
    ap.add_argument("--rex-svr", required=True)
    ap.add_argument("--clients", type=int, default=10)
    ap.add_argument("--interval", type=float, default=0.1)
    ap.add_argument("--duration", type=float, default=120)
    ap.add_argument("--workdir", help="scratch dir (default: a fresh temp dir)")
    ap.add_argument("--keep", action="store_true", help="keep the scratch dir afterwards")
    ap.add_argument("--summary", help="write the JSON summary here too")
    args = ap.parse_args()

    rex, svr = os.path.abspath(args.rex), os.path.abspath(args.rex_svr)
    root = args.workdir or tempfile.mkdtemp(prefix="rex-stress-")
    os.makedirs(root, exist_ok=True)
    log(f"workdir {root}")
    checks, summary = {}, {"clients": args.clients, "interval": args.interval, "duration": args.duration}

    server = Server(svr, root)
    summary["server_version"] = (server.health() or {}).get("version", "unknown")
    clients = [Client(i, rex, root, server.port) for i in range(args.clients)]
    try:
        # ── 1. seed ───────────────────────────────────────────────────
        seeder = clients[0]
        seeder.write("index.md", "---\ntitle: stress\n---\n\n# Stress report\n")
        seeder.write("sections/shared.md", "\n".join(f"line {i}: init" for i in range(args.clients)) + "\n")
        seeder.write("sections/hot.md", "# hot\n")
        for c in clients:
            c.write(f"notes/client-{c.idx}.md", f"# client {c.idx}\n")
        for c in clients:
            c.start()
        checks["seed_converged"] = wait_for(
            lambda: len(server.manifest()) == 3 + args.clients and not diverged(server, clients), 90, "seed convergence")
        log(f"seeded: {len(server.manifest())} files, converged={checks['seed_converged']}")

        # ── 2. deterministic delete -> re-create (the issue's index.md bug) ──
        before = conflict_count(server, "index.md")
        os.remove(clients[0].path("index.md"))
        gone = wait_for(lambda: all(not os.path.exists(c.path("index.md")) for c in clients), 60, "index.md deletion")
        recreator = clients[min(5, args.clients - 1)]
        recreator.write("index.md", "---\ntitle: stress\n---\n\n# Re-created\n")
        propagated = wait_for(
            lambda: "index.md" in server.manifest() and not diverged(server, clients), 60, "index.md re-create")
        time.sleep(2)
        new_conflicts = conflict_count(server, "index.md") - before
        checks["delete_recreate_no_conflict"] = gone and propagated and new_conflicts == 0
        summary["delete_recreate"] = {"deleted_everywhere": gone, "recreate_propagated": propagated,
                                      "conflicts_created": new_conflicts}
        log(f"delete->recreate: {summary['delete_recreate']}")

        # ── 3. chaos ──────────────────────────────────────────────────
        base_conflicts = conflict_count(server)
        log(f"chaos: {args.clients} clients x every {args.interval}s for {args.duration}s")
        t0 = time.time()
        summary["ops"] = chaos(clients, args.interval, args.duration)
        log(f"chaos done in {time.time() - t0:.0f}s: {summary['ops']}")

        # ── 4. quiesce + checks ───────────────────────────────────────
        settled = quiesce(server)
        summary["quiesce_seconds"] = settled
        alive = [c.idx for c in clients if c.proc.poll() is None]
        checks["all_processes_alive"] = server.proc.poll() is None and len(alive) == len(clients)
        by_kind = server.sql("SELECT kind, status, COALESCE(resolution,''), count(*) FROM conflicts WHERE id > ? "
                             "GROUP BY 1,2,3", (0,))
        summary["conflicts_by_kind_status"] = [list(r) for r in by_kind]
        summary["conflicts_during_chaos"] = conflict_count(server) - base_conflicts
        open_dupes = server.sql("SELECT path, local_username, count(*) FROM conflicts WHERE status='open' "
                                "GROUP BY 1,2 HAVING count(*) > 1")
        summary["open_conflicts"] = server.sql("SELECT count(*) FROM conflicts WHERE status='open'")[0][0]
        summary["max_open_per_path_user"] = max([r[2] for r in open_dupes], default=1 if summary["open_conflicts"] else 0)
        checks["open_conflicts_deduped"] = not open_dupes
        lost = []
        for c in clients:
            got = server.content(f"notes/client-{c.idx}.md")
            if c.last_note is not None and (got is None or got.decode() != c.last_note):
                lost.append(c.idx)
        summary["own_note_mismatch_clients"] = lost
        checks["no_lost_single_writer_writes"] = not lost
        shared = (server.content("sections/shared.md") or b"").decode(errors="replace")
        summary["shared_md_lines_touched"] = sum(
            1 for l in shared.split("\n") if l.startswith("line ") and not l.endswith(": init"))
        summary["merged_resolutions"] = server.sql(
            "SELECT count(*) FROM conflicts WHERE resolution='merge'")[0][0]
        summary["file_revisions"] = server.sql("SELECT count(*) FROM file_revisions")[0][0]
        health = server.health() or {}
        summary["health_after_chaos"] = health
        checks["git_materialization_drained"] = wait_for(
            lambda: (server.health() or {}).get("pending_materializations", 1) == 0, 60, "git flush")
        log(f"after chaos: open={summary['open_conflicts']} during_chaos={summary['conflicts_during_chaos']} "
            f"merged={summary['merged_resolutions']} lost={lost}")

        # ── 5. manageability: resolve everything, then converge ──────
        t0, rounds = time.time(), 0
        by_user = {c.user: c for c in clients}
        resolve_failures = []
        while rounds < 5:
            rows = server.sql("SELECT id, path, local_username, kind FROM conflicts WHERE status='open'")
            if not rows:
                break
            rounds += 1
            for cid, path, user, kind in rows:
                c = by_user.get(user)
                if c is None:
                    continue
                r = c.run("resolve", path, "--remote", "--id", str(cid))
                if r.returncode != 0 and "already resolved" not in r.stderr:
                    resolve_failures.append(f"{cid} {path} {user}: {r.stderr.strip()[-200:]}")
            time.sleep(3)
        summary["resolve_rounds"] = rounds
        summary["resolve_failures"] = resolve_failures[:20]
        converged = wait_for(lambda: not diverged(server, clients), 120, "final convergence")
        summary["converge_seconds"] = round(time.time() - t0, 1)
        summary["diverged_clients"] = diverged(server, clients)
        summary["open_conflicts_final"] = server.sql("SELECT count(*) FROM conflicts WHERE status='open'")[0][0]
        checks["converged_after_resolve"] = converged and summary["open_conflicts_final"] == 0
        log(f"resolve: rounds={rounds} failures={len(resolve_failures)} converged={converged} "
            f"in {summary['converge_seconds']}s")
    finally:
        for c in clients:
            if c.proc and c.proc.poll() is None:
                c.proc.terminate()
        server.proc.terminate()
        for p in [server.proc] + [c.proc for c in clients if c.proc]:
            try:
                p.wait(timeout=10)
            except subprocess.TimeoutExpired:
                p.kill()

    summary["checks"] = checks
    out = json.dumps(summary, indent=2)
    print(out)
    if args.summary:
        with open(args.summary, "w") as f:
            f.write(out)
    for name, ok in checks.items():
        print(f"{'PASS' if ok else 'FAIL'}  {name}")
    if not args.keep and not args.workdir:
        shutil.rmtree(root, ignore_errors=True)
    sys.exit(0 if all(checks.values()) else 1)


if __name__ == "__main__":
    main()
