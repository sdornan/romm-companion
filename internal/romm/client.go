// Package romm is a minimal client for the parts of RomM's REST API the
// companion uses: pairing, device registration, file download, play sessions,
// the emulator core map, and the shortcut queue proposed in docs/DESIGN.md.
//
// Routes marked "proposed" do not exist in RomM yet; they are the server
// contract this project is built against.
package romm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClientName is sent as the device client identifier and User-Agent.
const ClientName = "steam-companion"

// Client talks to one RomM server with one client token.
type Client struct {
	BaseURL string
	Token   string
	Version string
	HTTP    *http.Client
}

// New returns a client for baseURL using token.
func New(baseURL, token, version string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		Version: version,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// APIError is a non-2xx response.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("romm: HTTP %d: %s", e.Status, strings.TrimSpace(e.Body))
}

// ---- pairing ----

// ExchangeResponse is RomM's ClientTokenCreateSchema, trimmed to what we keep.
type ExchangeResponse struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Scopes   []string `json:"scopes"`
	RawToken string   `json:"raw_token"`
}

// ExchangePairCode trades a code from RomM's /pair page for a client token.
// It is the one unauthenticated call, so it works on a client with no token.
func ExchangePairCode(ctx context.Context, baseURL, code string) (*ExchangeResponse, error) {
	c := New(baseURL, "", "")
	var out ExchangeResponse
	body := map[string]string{"code": strings.ToUpper(strings.ReplaceAll(code, "-", ""))}
	if err := c.do(ctx, http.MethodPost, "/api/client-tokens/exchange", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- devices ----

// DeviceCreate is RomM's DeviceCreatePayload.
type DeviceCreate struct {
	Name          string `json:"name,omitempty"`
	Platform      string `json:"platform,omitempty"`
	Client        string `json:"client,omitempty"`
	ClientVersion string `json:"client_version,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	MACAddress    string `json:"mac_address,omitempty"`
	AllowExisting bool   `json:"allow_existing"`
}

// DeviceCreated is RomM's DeviceCreateResponse.
type DeviceCreated struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
}

// RegisterDevice creates or reuses a device record for this machine.
func (c *Client) RegisterDevice(ctx context.Context, d DeviceCreate) (*DeviceCreated, error) {
	var out DeviceCreated
	if err := c.do(ctx, http.MethodPost, "/api/devices", d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReportCapabilities sends the per-platform launch map (proposed field
// launch_capabilities on PUT /api/devices/{id}).
func (c *Client) ReportCapabilities(ctx context.Context, deviceID string, caps map[string]*string) error {
	body := map[string]any{"launch_capabilities": caps}
	return c.do(ctx, http.MethodPut, "/api/devices/"+url.PathEscape(deviceID), body, nil)
}

// ---- server config ----

// ServerConfig is RomM's ConfigResponse, trimmed to the emulator core map.
type ServerConfig struct {
	EJSCores        map[string][]string `json:"EJS_CORES"`
	EJSNightlyCores map[string][]string `json:"EJS_NIGHTLY_CORES"`
}

// GetConfig fetches the server's public configuration.
func (c *Client) GetConfig(ctx context.Context) (*ServerConfig, error) {
	var out ServerConfig
	if err := c.do(ctx, http.MethodGet, "/api/config", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- roms and files ----

// RomFile is the subset of RomM's RomFileSchema needed to download.
type RomFile struct {
	ID       int    `json:"id"`
	FileName string `json:"file_name"`
	FullPath string `json:"full_path"`
	Category string `json:"category"`
}

// Rom is the subset of RomM's rom schema the companion reads.
type Rom struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	PlatformSlug string    `json:"platform_slug"`
	FsName       string    `json:"fs_name"`
	FsNameNoExt  string    `json:"fs_name_no_ext"`
	Multi        bool      `json:"multi"`
	Files        []RomFile `json:"files"`
	PathCoverL   string    `json:"path_cover_large"`
	SGDBID       int       `json:"sgdb_id"`
}

// Title is what the shortcut is called in Steam.
func (r *Rom) Title() string {
	if r.Name != "" {
		return r.Name
	}
	if r.FsNameNoExt != "" {
		return r.FsNameNoExt
	}
	return r.FsName
}

// GetRom fetches one ROM by id.
func (c *Client) GetRom(ctx context.Context, id int) (*Rom, error) {
	var out Rom
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/roms/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadFile streams one ROM file to destDir, returning the local path. It
// writes to a temporary name and renames on success so a partial download is
// never mistaken for a complete one.
func (c *Client) DownloadFile(ctx context.Context, f RomFile, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, filepath.Base(f.FileName))
	if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
		return dest, nil
	}
	path := fmt.Sprintf("/api/roms/%d/files/content/%s", f.ID, url.PathEscape(f.FileName))
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", &APIError{Status: resp.StatusCode, Body: string(b)}
	}
	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return dest, os.Rename(tmp, dest)
}

// resourcesPath is where the web server exposes cover art, as a sibling of
// /api rather than a route on it.
const resourcesPath = "/assets/romm/resources/"

// DownloadCover fetches a rom's large cover. A rom with no cover, or a server
// that will not serve it, yields nil rather than an error: artwork is
// best-effort and must never fail an add.
func (c *Client) DownloadCover(ctx context.Context, rom *Rom) []byte {
	if rom.PathCoverL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.BaseURL+resourcesPath+rom.PathCoverL, nil,
	)
	if err != nil {
		return nil
	}
	// Covers are static files, so the token is usually ignored; sending it
	// costs nothing and covers deployments that do gate them.
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return c.readImage(req)
}

// readImage runs a request and returns the body, capping the read so a
// misrouted response cannot exhaust memory.
func (c *Client) readImage(req *http.Request) []byte {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil
	}
	return data
}

// ---- steam artwork ----

// SteamArtwork is RomM's SteamArtworkSchema: the two Steam library slots the
// ROM's own cover cannot fill. Either may be empty.
type SteamArtwork struct {
	URLHero string `json:"url_hero"`
	URLLogo string `json:"url_logo"`
}

// GetSteamArtwork asks RomM for a ROM's hero and logo. RomM holds the
// SteamGridDB key, so the companion never needs one.
func (c *Client) GetSteamArtwork(ctx context.Context, romID int) (*SteamArtwork, error) {
	var out SteamArtwork
	path := fmt.Sprintf("/api/shortcuts/artwork/%d", romID)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadImage fetches one artwork URL. Artwork is best-effort, so any
// failure yields nil rather than an error.
func (c *Client) DownloadImage(ctx context.Context, url string) []byte {
	if url == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", ClientName+"/"+c.Version)
	return c.readImage(req)
}

// ---- play sessions ----

// PlaySession is RomM's PlaySessionEntry.
type PlaySession struct {
	RomID      int       `json:"rom_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	DurationMS int64     `json:"duration_ms"`
}

// IngestPlaySessions posts finished sessions for this device.
func (c *Client) IngestPlaySessions(ctx context.Context, deviceID string, sessions []PlaySession) error {
	body := map[string]any{"device_id": deviceID, "sessions": sessions}
	return c.do(ctx, http.MethodPost, "/api/play-sessions", body, nil)
}

// ---- shortcut queue (proposed) ----

// Shortcut is a row in the proposed shortcuts table.
type Shortcut struct {
	ID         int     `json:"id"`
	RomID      int     `json:"rom_id"`
	DeviceID   string  `json:"device_id"`
	Status     string  `json:"status"`
	LaunchMode *string `json:"launch_mode"`
	SteamAppID *int64  `json:"steam_app_id"`
	Error      *string `json:"error"`
}

// PendingShortcuts returns this device's work queue.
func (c *Client) PendingShortcuts(ctx context.Context) ([]Shortcut, error) {
	var out []Shortcut
	q := "/api/shortcuts?device_id=me&status=pending_add,pending_remove,staged"
	if err := c.do(ctx, http.MethodGet, q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AckShortcut reports the outcome of processing one row.
func (c *Client) AckShortcut(ctx context.Context, id int, status string, steamAppID *uint32, errMsg string) error {
	body := map[string]any{"status": status}
	if steamAppID != nil {
		body["steam_app_id"] = *steamAppID
	}
	if errMsg != "" {
		body["error"] = errMsg
	}
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api/shortcuts/%d/ack", id), body, nil)
}

// ---- plumbing ----

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("User-Agent", ClientName+"/"+c.Version)
	return req, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &APIError{Status: resp.StatusCode, Body: string(b)}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
