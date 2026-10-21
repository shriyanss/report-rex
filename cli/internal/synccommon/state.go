package synccommon

// State classifies the synchronization status of a single path by
// comparing local content, the last-known-synced content, and the current
// remote content. Classification is always by hash/revision, never by
// filesystem modification time.
type State int

const (
	// StateUnchanged: local == last-synced == remote. No action.
	StateUnchanged State = iota
	// StateLocalOnlyChanged: local differs from last-synced, remote does
	// not. Push local state.
	StateLocalOnlyChanged
	// StateRemoteOnlyChanged: remote differs from last-synced, local does
	// not. Pull remote state.
	StateRemoteOnlyChanged
	// StateBothChanged: both local and remote diverged from last-synced.
	// Conflict/divergence handling required.
	StateBothChanged
	// StateInitialUnknownBase: no last-synced record exists for this path
	// (first sync) and local/remote both have content that differs. Treated
	// as an initial conflict rather than guessed.
	StateInitialUnknownBase
)

func (s State) String() string {
	switch s {
	case StateUnchanged:
		return "unchanged"
	case StateLocalOnlyChanged:
		return "local_only_changed"
	case StateRemoteOnlyChanged:
		return "remote_only_changed"
	case StateBothChanged:
		return "both_changed"
	case StateInitialUnknownBase:
		return "initial_unknown_base"
	default:
		return "unknown"
	}
}

// PathSnapshot describes what is known about one path from one side
// (local disk, or the client's last-synced record, or the server).
// A zero-value HashKnown=false / Exists=false means "path absent."
type PathSnapshot struct {
	Exists bool
	Hash   string // sha256 hex; meaningless if !Exists
}

// Classify determines the sync state of a path given:
//   - hasLastSync: whether the client has ever synced this path before
//   - lastSynced: the local snapshot as of the last successful sync
//   - local: the current on-disk snapshot
//   - remote: the current server snapshot
func Classify(hasLastSync bool, lastSynced, local, remote PathSnapshot) State {
	localChanged := !hasLastSync || !snapshotsEqual(lastSynced, local)
	remoteChanged := !hasLastSync || !snapshotsEqual(lastSynced, remote)

	if !hasLastSync {
		switch {
		case !local.Exists && !remote.Exists:
			return StateUnchanged
		case local.Exists && !remote.Exists:
			// Remote empty, local populated: push, not a conflict.
			return StateLocalOnlyChanged
		case !local.Exists && remote.Exists:
			// Remote populated, local empty: pull, not a conflict.
			return StateRemoteOnlyChanged
		default:
			// Both exist with no known base: only a real conflict if their
			// content actually differs.
			if snapshotsEqual(local, remote) {
				return StateUnchanged
			}
			return StateInitialUnknownBase
		}
	}

	switch {
	case !localChanged && !remoteChanged:
		return StateUnchanged
	case localChanged && !remoteChanged:
		return StateLocalOnlyChanged
	case !localChanged && remoteChanged:
		return StateRemoteOnlyChanged
	default:
		// Both changed. If they converged to the same content, treat as
		// unchanged (no transfer needed) rather than a false conflict.
		if snapshotsEqual(local, remote) {
			return StateUnchanged
		}
		return StateBothChanged
	}
}

func snapshotsEqual(a, b PathSnapshot) bool {
	if a.Exists != b.Exists {
		return false
	}
	if !a.Exists {
		return true
	}
	return a.Hash == b.Hash
}
