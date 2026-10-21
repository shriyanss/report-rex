import { RexConfig } from "./config";

// tlsArgs returns the --insecure/--ca-cert flag(s) rex sync/presence need to
// trust rex-svr's self-signed certificate. insecure wins if both are set.
//
// Kept out of config.ts: RexConfig is used here only as a type (elided from
// the compiled output), so requiring this module never pulls in "vscode" --
// unlike config.ts, which imports it at runtime. That keeps this testable
// with plain node:test outside the extension host.
export function tlsArgs(cfg: Pick<RexConfig, "insecure" | "caCert">): string[] {
  if (cfg.insecure) {
    return ["--insecure"];
  }
  if (cfg.caCert) {
    return ["--ca-cert", cfg.caCert];
  }
  return [];
}
