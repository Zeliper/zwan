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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Zeliper/zwan/shared/certpin"
	"github.com/Zeliper/zwan/shared/keys"
	"github.com/Zeliper/zwan/shared/proto"
)

const requestTimeout = 10 * time.Second

// Result is the outcome of a successful join. Private is kept so the caller can
// bring up a wireguard-go device with the same key it registered.
type Result struct {
	Register  proto.RegisterResponse
	Peers     []proto.Peer
	Private   keys.Private
	PublicKey string
}

// Client talks to one control server over the control channel.
type Client struct {
	base string
	pin  string
	http *http.Client
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

// Join registers this device and returns the assigned identity plus the current
// peer list. endpoint is the host:port peers can send WireGuard traffic to (may
// be empty for a control-plane-only join).
func (c *Client) Join(token, deviceUUID, hostname, endpoint string) (*Result, error) {
	priv, err := keys.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate node key: %w", err)
	}
	pub := priv.Public().String()

	body, _ := json.Marshal(proto.RegisterRequest{
		Token:      token,
		DeviceUUID: deviceUUID,
		Hostname:   hostname,
		PublicKey:  pub,
		Endpoint:   endpoint,
	})
	var reg proto.RegisterResponse
	if err := c.post("/v1/register", body, &reg); err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	peers, err := c.Peers()
	if err != nil {
		return nil, fmt.Errorf("fetch peers: %w", err)
	}
	return &Result{Register: reg, Peers: peers, Private: priv, PublicKey: pub}, nil
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

// PublishService registers (or updates) a service in the network.
func (c *Client) PublishService(token string, svc proto.Service) error {
	body, _ := json.Marshal(proto.RegisterServiceRequest{Token: token, Service: svc})
	var out proto.Service
	return c.post("/v1/services", body, &out)
}

func (c *Client) post(path string, body []byte, out any) error {
	resp, err := c.http.Post(c.base+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func (c *Client) get(path string, out any) error {
	resp, err := c.http.Get(c.base + path)
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
		if e.Error != "" {
			return fmt.Errorf("server %d: %s", resp.StatusCode, e.Error)
		}
		return fmt.Errorf("server %d", resp.StatusCode)
	}
	return json.Unmarshal(data, out)
}
