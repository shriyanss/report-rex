package synccommon

import "testing"

func TestIsExcluded(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"index.md", false},
		{"assets/screenshots/login.png", false},
		{".git/config", true},
		{"nested/.git/HEAD", true},
		{".report-rex/sync.db", true},
		{"assets/.DS_Store", true},
		{"findings/SQLI-001.md.swp", true},
		{"findings/SQLI-001.md.swo", true},
		{"tmp/upload.tmp", true},
		{"reports/vuln-1.md", false},
		{".vscode/settings.json", true},
		{"notes/.vscode-like.md", false},
		{"rex-svr.db", true},
		{"rex-svr.db-wal", true},
		{"rex-svr.db-shm", true},
		{"rex-svr.key", true},
		{"rex-svr.crt", true},
		{"rex-svr-git-workspace/index.md", true},
		{"artifacts/loot/server.key", false},
		{"notes/rex-svr.md", false},
	}

	for _, c := range cases {
		got := IsExcluded(c.in)
		if got != c.want {
			t.Errorf("IsExcluded(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
