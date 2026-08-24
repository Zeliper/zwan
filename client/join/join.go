// Package join implements the agent-side control-plane flow: register a node
// key with the control server and fetch the peer and service lists.
//
// A control server is addressed by URL and, when it has no public domain, by the
// SPKI pin of its self-signed certificate (see shared/certpin). Both can travel
// as a single string: "https://203.0.113.5:8787#sha256:<fingerprint>".
package join

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Zeliper/zwan/shared/certpin"
	"github.com/Zeliper/zwan/shared/keys"
	"github.com/Zeliper/zwan/shared/proto"
)

const requestTimeout = 10 * time.Second

// errUnauthenticated marks a 401 from the control plane, which means our node
// token is no longer recognised — normally because the server restarted and
// lost its in-memory state.
var errUnauthenticated = errors.New("control server rejected our node token")

// identity is everything a re-registration needs: the same node key and the
// same join credentials, so refreshing a token never changes who this device is.
type identity struct {
	token      string
	deviceUUID string
	hostname   string
	endpoint   string
	priv       keys.Private
	pub        string
	assignedIP string
}

// Result is the outcome of a successful join. Private is kept so the caller can
// bring up a wireguard-go device with the same key it registered.
type Result struct {
	Register  proto.RegisterResponse
	Peers     []proto.Peer
	Private   keys.Private
	PublicKey string
}

// Client talks to one control server over the control channel.
//
// Join exchanges the shared join token for a per-device node token, which the
// client then presents on every later call; the join token is never sent again.
type Client struct {
	base string
	pin  string
	http *http.Client

	mu        sync.Mutex
	nodeToken string
	id        *identity
}

// NewClient builds a control-plane client for serverURL.
//
// serverURL may carry the server's key pin as a fragment
// ("https://host:8787#sha256:..."); a non-empty pin argument overrides it. A URL
// without a scheme is treated as https, so a plaintext server must be addressed
// explicitly as "http://...".
func NewClient(serverURL, pin string) (*Client, error) {
	base, embedded := SplitPin(serverURL)
	if pin == "" {
		pin = embedded
	}
	base, err := NormalizeURL(base)
	if err != nil {
		return nil, err
	}
	if pin != "" {
		if pin, err = certpin.Normalize(pin); err != nil {
			return nil, fmt.Errorf("server pin: %w", err)
		}
	}
	hc, err := certpin.HTTPClient(pin, requestTimeout)
	if err != nil {
		return nil, err
	}
	return &Client{base: base, pin: pin, http: hc}, nil
}

// BaseURL is the normalized control server URL, without the pin.
func (c *Client) BaseURL() string { return c.base }

// Pin is the canonical server pin in use, or "" when the system trust store is.
func (c *Client) Pin() string { return c.pin }

// Authenticated reports whether the client holds a node token, i.e. has joined.
func (c *Client) Authenticated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nodeToken != ""
}

func (c *Client) setNodeToken(tok string) {
	c.mu.Lock()
	c.nodeToken = tok
	c.mu.Unlock()
}

func (c *Client) token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nodeToken
}

// Join registers this device and returns the assigned identity plus the current
// peer list. endpoint is the host:port peers can send WireGuard traffic to (may
// be empty for a control-plane-only join).
func (c *Client) Join(token, deviceUUID, hostname, endpoint string) (*Result, error) {
	priv, err := keys.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate node key: %w", err)
	}
	id := &identity{
		token: token, deviceUUID: deviceUUID, hostname: hostname, endpoint: endpoint,
		priv: priv, pub: priv.Public().String(),
	}
	reg, err := c.register(id)
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	id.assignedIP = reg.AssignedIP
	c.mu.Lock()
	c.id = id
	c.mu.Unlock()

	peers, err := c.Peers()
	if err != nil {
		return nil, fmt.Errorf("fetch peers: %w", err)
	}
	return &Result{Register: reg, Peers: peers, Private: priv, PublicKey: id.pub}, nil
}

// register performs one /v1/register round trip and adopts the node token it
// returns.
func (c *Client) register(id *identity) (proto.RegisterResponse, error) {
	body, _ := json.Marshal(proto.RegisterRequest{
		Token:      id.token,
		DeviceUUID: id.deviceUUID,
		Hostname:   id.hostname,
		PublicKey:  id.pub,
		Endpoint:   id.endpoint,
	})
	var reg proto.RegisterResponse
	if err := c.do(http.MethodPost, "/v1/register", body, &reg, false); err != nil {
		return reg, err
	}
	if reg.NodeToken == "" {
		return reg, fmt.Errorf("server issued no node token (is it running an older version?)")
	}
	c.setNodeToken(reg.NodeToken)
	return reg, nil
}

// reauthenticate re-registers this device after the server stopped recognising
// our node token. It reuses the same node key, so the tunnel is untouched. If
// the server hands back a different overlay address the tunnel really is stale,
// and saying so beats silently running on an address nobody routes to.
func (c *Client) reauthenticate() error {
	c.mu.Lock()
	id := c.id
	c.mu.Unlock()
	if id == nil {
		return errUnauthenticated
	}
	reg, err := c.register(id)
	if err != nil {
		return fmt.Errorf("re-register: %w", err)
	}
	if reg.AssignedIP != id.assignedIP {
		return fmt.Errorf("control server reassigned this device from %s to %s; reconnect to pick it up",
			id.assignedIP, reg.AssignedIP)
	}
	return nil
}

// Peers returns the current members of the network.
func (c *Client) Peers() ([]proto.Peer, error) {
	var out proto.PeersResponse
	if err := c.get("/v1/peers", &out); err != nil {
		return nil, err
	}
	return out.Peers, nil
}

// Services returns the current services of the network.
func (c *Client) Services() ([]proto.Service, error) {
	var out proto.ServicesResponse
	if err := c.get("/v1/services", &out); err != nil {
		return nil, err
	}
	return out.Services, nil
}

// PublishService registers (or updates) a service hosted on this node. Join
// first: the call is authenticated with the node token.
func (c *Client) PublishService(svc proto.Service) error {
	body, _ := json.Marshal(proto.RegisterServiceRequest{Service: svc})
	var out proto.Service
	return c.post("/v1/services", body, &out)
}

func (c *Client) post(path string, body []byte, out any) error {
	return c.do(http.MethodPost, path, body, out, true)
}

func (c *Client) get(path string, out any) error {
	return c.do(http.MethodGet, path, nil, out, true)
}

// do sends one request, and on a 401 re-registers once and sends it again, so a
// server restart does not strand a running connection.
func (c *Client) do(method, path string, body []byte, out any, retryAuth bool) error {
	err := c.roundTrip(method, path, body, out)
	if err == nil || !retryAuth || !errors.Is(err, errUnauthenticated) {
		return err
	}
	if err := c.reauthenticate(); err != nil {
		return err
	}
	return c.roundTrip(method, path, body, out)
}

func (c *Client) roundTrip(method, path string, body []byte, out any) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok := c.token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

// SplitPin separates a "<url>#<pin>" join string into its two parts. A string
// without a fragment yields an empty pin.
func SplitPin(s string) (server, pin string) {
	s = strings.TrimSpace(s)
	i := strings.LastIndexByte(s, '#')
	if i < 0 {
		return s, ""
	}
	pin = s[i+1:]
	if un, err := url.PathUnescape(pin); err == nil {
		pin = un
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(pin)
}

// NormalizeURL cleans a user-entered server address: it defaults to https when
// no scheme is given and strips any trailing slash and path.
func NormalizeURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("server address is empty")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("server address %q: %w", s, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("server address %q: unsupported scheme %q", s, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("server address %q has no host", s)
	}
	return u.Scheme + "://" + u.Host, nil
}

func decode(resp *http.Response, out any) error {
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var e proto.ErrorResponse
		_ = json.Unmarshal(data, &e)
		msg := e.Error
		if msg == "" {
			msg = resp.Status
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("%w: %s", errUnauthenticated, msg)
		}
		return fmt.Errorf("server %d: %s", resp.StatusCode, msg)
	}
	return json.Unmarshal(data, out)
}
