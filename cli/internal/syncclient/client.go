package syncclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shriyanss/report-rex/cli/internal/synccommon"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// ErrConflict is returned by Put/Delete when the server rejects the
// change as a divergent/stale update. Detail carries the server's
// conflict record.
type ErrConflict struct {
	Detail syncproto.ConflictError
}

func (e *ErrConflict) Error() string {
	return fmt.Sprintf("conflict on upload: conflict_id=%d kind=%s remote_revision=%d", e.Detail.ConflictID, e.Detail.Kind, e.Detail.RemoteRevision)
}

// Client is a thin HTTP client for the sync server's REST API.
type Client struct {
	baseURL string
	http    *http.Client

	mu                 sync.Mutex
	token              string
	username, password string // kept from Login for re-login on 401
}

func NewClient(host string, port int, tlsConfig *tls.Config) *Client {
	c := &Client{baseURL: fmt.Sprintf("https://%s:%d/api/v1", host, port)}
	c.http = &http.Client{
		Timeout:   30 * time.Second,
		Transport: &reauth{c: c, next: &http.Transport{TLSClientConfig: tlsConfig}},
	}
	return c
}

func (c *Client) SetToken(token string) { c.mu.Lock(); c.token = token; c.mu.Unlock() }
func (c *Client) Token() string         { c.mu.Lock(); defer c.mu.Unlock(); return c.token }

// Relogin logs in again with the credentials of the last successful Login.
func (c *Client) Relogin(ctx context.Context) error {
	c.mu.Lock()
	username, password := c.username, c.password
	c.mu.Unlock()
	if password == "" {
		return fmt.Errorf("not logged in")
	}
	_, err := c.Login(ctx, username, password)
	return err
}

// reauth retries a request once after a fresh login when the server answers
// 401. rex-svr keeps sessions in its database, so once it's restarted on a
// fresh one every earlier token is unknown, and a long-running `rex sync`,
// `rex presence` or `rex mcp` would otherwise fail every request until
// restarted by hand (issue #62).
type reauth struct {
	c    *Client
	next http.RoundTripper
}

func (t *reauth) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || strings.HasSuffix(req.URL.Path, "/auth/login") ||
		(req.Body != nil && req.GetBody == nil) || t.c.Relogin(req.Context()) != nil {
		return resp, err
	}
	resp.Body.Close()
	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		if retry.Body, err = req.GetBody(); err != nil {
			return nil, err
		}
	}
	retry.Header.Set("Authorization", "Bearer "+t.c.Token())
	return t.next.RoundTrip(retry)
}

func (c *Client) Login(ctx context.Context, username, password string) (string, error) {
	body, _ := json.Marshal(syncproto.LoginRequest{Username: username, Password: password})
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed: %s", readErrorBody(resp))
	}
	var lr syncproto.LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return "", err
	}
	c.mu.Lock()
	c.token, c.username, c.password = lr.Token, username, password
	c.mu.Unlock()
	return lr.Token, nil
}

func (c *Client) Manifest(ctx context.Context) ([]syncproto.FileEntry, error) {
	var mr syncproto.ManifestResponse
	if err := c.doJSON(ctx, "GET", "/manifest", nil, nil, &mr); err != nil {
		return nil, err
	}
	return mr.Files, nil
}

// Presence returns a REST snapshot of every currently-active editor-presence
// lease, for a caller that just connected and hasn't seen a WS presence
// broadcast yet. Empty rather than an error if the server has no presence
// hub configured, since presence is purely advisory.
func (c *Client) Presence(ctx context.Context) ([]syncproto.FilePresenceEntry, error) {
	var pr syncproto.PresenceListResponse
	if err := c.doJSON(ctx, "GET", "/presence", nil, nil, &pr); err != nil {
		return nil, err
	}
	return pr.Presence, nil
}

// GetFile fetches the current content of path. ok=false with err=nil means
// the path is tombstoned (deleted) on the server.
func (c *Client) GetFile(ctx context.Context, path string) (content []byte, revision int64, sha256 string, ok bool, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/files/"+path, nil)
	if err != nil {
		return nil, 0, "", false, err
	}
	c.authorize(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusGone {
		return nil, 0, "", false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, "", false, fmt.Errorf("get %s: %s", path, readErrorBody(resp))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, "", false, err
	}
	rev, _ := strconv.ParseInt(resp.Header.Get(syncproto.HeaderRevision), 10, 64)
	return body, rev, resp.Header.Get(syncproto.HeaderSHA256), true, nil
}

// PutFile uploads content for path. baseRevision is nil for a brand-new
// path. Returns *ErrConflict if the server rejects it as divergent.
func (c *Client) PutFile(ctx context.Context, path string, content []byte, baseRevision *int64) (revision int64, sha256 string, err error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/files/"+path, bytes.NewReader(content))
	if err != nil {
		return 0, "", err
	}
	c.authorize(req)
	if baseRevision != nil {
		req.Header.Set(syncproto.HeaderBaseRevision, strconv.FormatInt(*baseRevision, 10))
	}
	req.Header.Set(syncproto.HeaderSHA256, synccommon.HashBytes(content))

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		var ce syncproto.ConflictError
		json.NewDecoder(resp.Body).Decode(&ce)
		return 0, "", &ErrConflict{Detail: ce}
	}
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("put %s: %s", path, readErrorBody(resp))
	}
	var pr syncproto.PutFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return 0, "", err
	}
	return pr.Revision, pr.SHA256, nil
}

func (c *Client) DeleteFile(ctx context.Context, path string, baseRevision int64) (revision int64, err error) {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+"/files/"+path, nil)
	if err != nil {
		return 0, err
	}
	c.authorize(req)
	req.Header.Set(syncproto.HeaderBaseRevision, strconv.FormatInt(baseRevision, 10))

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		var ce syncproto.ConflictError
		json.NewDecoder(resp.Body).Decode(&ce)
		return 0, &ErrConflict{Detail: ce}
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("delete %s: %s", path, readErrorBody(resp))
	}
	var dr syncproto.DeleteFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		return 0, err
	}
	return dr.Revision, nil
}

func (c *Client) ListConflicts(ctx context.Context) ([]syncproto.ConflictSummary, error) {
	var cr syncproto.ConflictsResponse
	if err := c.doJSON(ctx, "GET", "/conflicts", nil, nil, &cr); err != nil {
		return nil, err
	}
	return cr.Conflicts, nil
}

func (c *Client) GetConflict(ctx context.Context, id int64, includeLocal, includeRemote, includeBase bool) (*syncproto.ConflictDetail, error) {
	path := fmt.Sprintf("/conflicts/%d", id)
	var parts []string
	if includeLocal {
		parts = append(parts, "local")
	}
	if includeRemote {
		parts = append(parts, "remote")
	}
	if includeBase {
		parts = append(parts, "base")
	}
	q := ""
	for i, p := range parts {
		if i == 0 {
			q = "?include=" + p
		} else {
			q += "," + p
		}
	}
	var detail syncproto.ConflictDetail
	if err := c.doJSON(ctx, "GET", path+q, nil, nil, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

func (c *Client) ResolveConflict(ctx context.Context, id int64, req syncproto.ResolveRequest) (*syncproto.ResolveResponse, error) {
	body, _ := json.Marshal(req)
	var resp syncproto.ResolveResponse
	if err := c.doJSON(ctx, "POST", fmt.Sprintf("/conflicts/%d/resolve", id), bytes.NewReader(body), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListHandoffs returns the handoffs in one scope. user names another
// operator's queue (empty means the authenticated user's own); all overrides
// user and returns everyone's. Resolved handoffs are excluded unless asked
// for. Handoffs whose directory and `to` metadata disagree come back
// separately, so callers can surface them rather than silently dropping a
// misassigned file.
func (c *Client) ListHandoffs(ctx context.Context, user string, all, resolved bool) ([]syncproto.HandoffEntry, []syncproto.HandoffInvalid, error) {
	q := url.Values{}
	if all {
		q.Set("all", "1")
	} else if user != "" {
		q.Set("user", user)
	}
	if resolved {
		q.Set("resolved", "1")
	}
	path := "/handoffs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var lr syncproto.HandoffListResponse
	if err := c.doJSON(ctx, "GET", path, nil, nil, &lr); err != nil {
		return nil, nil, err
	}
	return lr.Handoffs, lr.Invalid, nil
}

// GetHandoff fetches one handoff by its stable id, including its content, so
// it can be read without being synchronized into the local workspace. Works
// for resolved handoffs too.
func (c *Client) GetHandoff(ctx context.Context, id string) (*syncproto.HandoffDetail, error) {
	var d syncproto.HandoffDetail
	if err := c.doJSON(ctx, "GET", "/handoffs/"+url.PathEscape(id), nil, nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// ResolveHandoff marks a handoff resolved. The server records the final
// metadata and retires the active path; the canonical content is retained
// server-side for history.
func (c *Client) ResolveHandoff(ctx context.Context, id string) (*syncproto.HandoffResolveResponse, error) {
	var rr syncproto.HandoffResolveResponse
	if err := c.doJSON(ctx, "POST", "/handoffs/"+url.PathEscape(id)+"/resolve", nil, nil, &rr); err != nil {
		return nil, err
	}
	return &rr, nil
}

func (c *Client) Healthz(ctx context.Context) (*syncproto.HealthResponse, error) {
	var hr syncproto.HealthResponse
	if err := c.doJSON(ctx, "GET", "/healthz", nil, nil, &hr); err != nil {
		return nil, err
	}
	return &hr, nil
}

func (c *Client) authorize(req *http.Request) {
	if token := c.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// RevertPreview asks what reverting the workspace to right after path
// reached revision would change, without changing anything.
func (c *Client) RevertPreview(ctx context.Context, req syncproto.RevertRequest) (*syncproto.RevertPreview, error) {
	body, _ := json.Marshal(req)
	var p syncproto.RevertPreview
	if err := c.doJSON(ctx, "POST", "/revert/preview", bytes.NewReader(body), nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Revert applies a previewed revert; req.Plan must be the preview's token.
func (c *Client) Revert(ctx context.Context, req syncproto.RevertRequest) (*syncproto.RevertResponse, error) {
	body, _ := json.Marshal(req)
	var r syncproto.RevertResponse
	if err := c.doJSON(ctx, "POST", "/revert", bytes.NewReader(body), nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// BulkDelete tombstones req.Paths on the server in one admin-gated call.
func (c *Client) BulkDelete(ctx context.Context, req syncproto.BulkDeleteRequest) (*syncproto.BulkDeleteResponse, error) {
	body, _ := json.Marshal(req)
	var r syncproto.BulkDeleteResponse
	if err := c.doJSON(ctx, "POST", "/admin/delete", bytes.NewReader(body), nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// VerifyAdmin checks password against the server's admin password.
func (c *Client) VerifyAdmin(ctx context.Context, password string) error {
	body, _ := json.Marshal(syncproto.AdminVerifyRequest{AdminPassword: password})
	return c.doJSON(ctx, "POST", "/admin/verify", bytes.NewReader(body), nil, nil)
}

// Blame returns who last changed each line of path, per the server's git history.
func (c *Client) Blame(ctx context.Context, path string) (*syncproto.BlameResponse, error) {
	var b syncproto.BlameResponse
	if err := c.doJSON(ctx, "GET", "/blame/"+path, nil, nil, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body io.Reader, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	c.authorize(req)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", method, path, readErrorBody(resp))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func readErrorBody(resp *http.Response) string {
	var er syncproto.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err == nil && er.Error != "" {
		return fmt.Sprintf("%d %s: %s", resp.StatusCode, er.Code, er.Error)
	}
	return resp.Status
}
