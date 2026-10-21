import { test } from "node:test";
import * as assert from "node:assert";
import { parseLine } from "../src/presenceBridge";

test("parseLine decodes a valid presence event", () => {
  const ev = parseLine(
    '{"type":"presence","username":"alice","session_id":"s1","file":"reports/vuln-1.md","state":"editing","dirty":true,"action":"active"}'
  );
  assert.deepStrictEqual(ev, {
    type: "presence",
    username: "alice",
    session_id: "s1",
    file: "reports/vuln-1.md",
    state: "editing",
    dirty: true,
    action: "active",
  });
});

test("parseLine decodes a valid status event", () => {
  const ev = parseLine('{"type":"status","connected":false,"error":"dial tcp: refused"}');
  assert.deepStrictEqual(ev, { type: "status", connected: false, error: "dial tcp: refused" });
});

test("parseLine drops an empty line", () => {
  assert.strictEqual(parseLine(""), null);
});

test("parseLine drops malformed JSON", () => {
  assert.strictEqual(parseLine("not json"), null);
});

test("parseLine drops JSON without a type field", () => {
  assert.strictEqual(parseLine('{"foo":"bar"}'), null);
});

test("parseLine drops a bare JSON scalar", () => {
  assert.strictEqual(parseLine("42"), null);
});
