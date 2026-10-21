import { PresenceEvent } from "./presenceBridge";

export interface RemoteEntry {
  username: string;
  state: string;
  dirty: boolean;
}

// remote[file][session_id] = {username, state, dirty} -- mirrors init.lua's
// `remote` table exactly.
export type RemoteState = Map<string, Map<string, RemoteEntry>>;

export function createRemoteState(): RemoteState {
  return new Map();
}

// applyPresenceEvent mutates `remote` in place per one incoming "presence"
// event: an "idle" action clears that session, anything else (re)sets it.
export function applyPresenceEvent(remote: RemoteState, ev: PresenceEvent): void {
  let byFile = remote.get(ev.file);
  if (!byFile) {
    byFile = new Map();
    remote.set(ev.file, byFile);
  }
  if (ev.action === "idle") {
    byFile.delete(ev.session_id);
  } else {
    byFile.set(ev.session_id, { username: ev.username, state: ev.state, dirty: ev.dirty });
  }
}

// othersOn returns every remote entry on `file` excluding `ownUsername` --
// the server broadcasts every session's updates to every connection,
// including the sender's own. Mirrors statusline.lua's others_on.
export function othersOn(remote: RemoteState, file: string, ownUsername: string): RemoteEntry[] {
  const byFile = remote.get(file);
  if (!byFile) {
    return [];
  }
  return Array.from(byFile.values()).filter((e) => e.username !== ownUsername);
}

// isConcurrentEdit reports whether `file` already has another session in
// the "editing" state -- the trigger for the stronger warning. Mirrors
// statusline.lua's is_concurrent_edit.
export function isConcurrentEdit(remote: RemoteState, file: string, ownUsername: string): string | null {
  const editor = othersOn(remote, file, ownUsername).find((e) => e.state === "editing");
  return editor ? editor.username : null;
}

// statusText returns a short status-bar fragment for `file`, or "" if no one
// else is present on it. Editing takes precedence over viewing. Mirrors
// statusline.lua's status_text.
export function statusText(remote: RemoteState, file: string, ownUsername: string): string {
  const others = othersOn(remote, file, ownUsername);
  if (others.length === 0) {
    return "";
  }
  const editing = others.find((e) => e.state === "editing");
  const who = editing ?? others[0];
  const verb = editing ? "editing" : "viewing";
  return `⚠ ${who.username} ${verb}`;
}

// isEditing reports whether any other session is currently editing `file` --
// drives the FileDecorationProvider badge.
export function isEditing(remote: RemoteState, file: string, ownUsername: string): RemoteEntry | null {
  return othersOn(remote, file, ownUsername).find((e) => e.state === "editing") ?? null;
}
