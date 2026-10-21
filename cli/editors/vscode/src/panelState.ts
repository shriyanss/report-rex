// Pure logic for the Report Rex "Sync & Server" view (issues #38, #42) --
// no `vscode` import, so this stays runnable under plain `node --test`.

import { StatusReport } from "./cli";
import { relativeTime } from "./model";
import { PHASE_LABEL, SyncState } from "./syncState";

export interface PanelState {
  sync: SyncState;
  syncEnabled: boolean;
  presenceConnected: boolean | undefined; // undefined = no bridge running
  health?: StatusReport;
  healthCheckedAt?: string;
  myConflicts: number;
  otherConflicts: number;
  host: string;
  port: number;
  username: string;
  dir: string;
  passwordSet: boolean;
  caCert: string;
  insecure: boolean;
  lintEnabled: boolean;
}

export interface PanelRow {
  label: string;
  description?: string;
  tooltip?: string;
  icon: string; // codicon id
  color?: string; // theme color id
  command?: string;
}

const PHASE_ICON: Record<SyncState["phase"], string> = {
  stopped: "circle-slash",
  starting: "loading~spin",
  connected: "pass-filled",
  offline: "debug-disconnect",
  reconnecting: "loading~spin",
  error: "error",
};

function settingRow(label: string, value: string, command: string): PanelRow {
  return { label, description: value || "Not set", icon: value ? "gear" : "circle-large-outline", command };
}

function serverRow(s: PanelState): PanelRow {
  const h = s.health;
  if (!h) {
    const live = s.presenceConnected;
    return {
      label: "Server",
      description: live === true ? "Online" : live === false ? "Offline" : "Unknown — click to test",
      icon: live === true ? "pass-filled" : live === false ? "error" : "circle-large-outline",
      command: "report-rex.testConnection",
    };
  }
  if (h.error) {
    return {
      label: "Server",
      description: `Failed (${h.hint ?? "error"})`,
      tooltip: `${h.error}\nChecked ${relativeTime(s.healthCheckedAt)} — click to test again`,
      icon: "error",
      color: "problemsErrorIcon.foreground",
      command: "report-rex.testConnection",
    };
  }
  const bits = [h.health?.status === "ok" ? "Online" : `Degraded (${h.health?.status})`];
  if (h.health?.version) {
    bits.push(`rex-svr ${h.health.version}`);
  }
  bits.push(`${h.latency_ms} ms`);
  return {
    label: "Server",
    description: bits.join(" · "),
    tooltip:
      `${h.server} as ${h.user}\n${h.files} files, git ${h.health?.git_ok ? "ok" : "FAILING"}, ` +
      `db ${h.health?.db_ok ? "ok" : "FAILING"}, ${h.health?.pending_materializations ?? 0} pending commits\n` +
      `Checked ${relativeTime(s.healthCheckedAt)} — click to test again`,
    icon: h.health?.status === "ok" ? "pass-filled" : "warning",
    command: "report-rex.testConnection",
  };
}

function tlsRow(s: PanelState): PanelRow {
  if (s.insecure) {
    return { label: "TLS", description: "Insecure (no verification)", icon: "unlock", command: "report-rex.configureTls" };
  }
  if (s.caCert) {
    return { label: "TLS", description: `Pinned: ${s.caCert}`, icon: "lock", command: "report-rex.configureTls" };
  }
  return {
    label: "TLS",
    description: "Not set — required",
    icon: "warning",
    color: "problemsWarningIcon.foreground",
    command: "report-rex.configureTls",
  };
}

// configured mirrors resolveConfig()'s requirement; the view shows its
// setup walkthrough (viewsWelcome) instead of rows until it holds.
export function configured(s: Pick<PanelState, "host" | "username">): boolean {
  return !!(s.host && s.username);
}

// panelRows renders the Sync & Server view -- separated from the
// TreeDataProvider so the label/icon branching is testable without an
// extension host.
export function panelRows(s: PanelState): PanelRow[] {
  const running = s.syncEnabled && s.sync.phase !== "stopped";
  const syncDesc =
    s.sync.phase === "stopped"
      ? "Stopped — click to start"
      : s.sync.lastError && s.sync.phase !== "connected"
      ? s.sync.lastError
      : `last change ${relativeTime(s.sync.lastSyncAt)}`;
  const conflicts = s.myConflicts + s.otherConflicts;
  return [
    {
      label: `Sync: ${PHASE_LABEL[s.sync.phase]}`,
      description: syncDesc,
      tooltip:
        `${running ? "Click to stop syncing this workspace" : "Click to test the connection and start syncing"}\n` +
        `This session: ${s.sync.pushed} pushed, ${s.sync.pulled} pulled, ${s.sync.merged} auto-merged, ${s.sync.conflicts} conflicts`,
      icon: PHASE_ICON[s.sync.phase],
      command: running ? "report-rex.stopSync" : "report-rex.startSync",
    },
    serverRow(s),
    {
      label: "Conflicts",
      description: conflicts === 0 ? "None" : `${s.myConflicts} yours, ${s.otherConflicts} teammates'`,
      icon: s.myConflicts > 0 ? "warning" : "check",
      color: s.myConflicts > 0 ? "problemsWarningIcon.foreground" : undefined,
      command: "reportRex.conflicts.focus",
    },
    settingRow("Host", s.host, "report-rex.setHost"),
    settingRow("Port", String(s.port), "report-rex.setPort"),
    settingRow("Username", s.username, "report-rex.setUsername"),
    settingRow("Directory", s.dir, "report-rex.setDir"),
    {
      label: "Password",
      description: s.passwordSet ? "Set" : "Not set",
      icon: s.passwordSet ? "key" : "shield",
      command: "report-rex.setPassword",
    },
    tlsRow(s),
    {
      label: "Linter",
      description: s.lintEnabled ? "On (validate on save)" : "Off",
      icon: s.lintEnabled ? "checklist" : "circle-slash",
      command: "report-rex.toggleLint",
    },
  ];
}
