// Package join implements the agent-side control-plane flow: register a node
// key with the control server and fetch the peer list.
package join

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Zeliper/zwan/shared/keys"
	"github.com/Zeliper/zwan/shared/proto"
)

// Result is the outcome of a successful join. Private is kept so the caller can
// bring up a wireguard-go device with the same key it registered.
type Result struct {
	Register  proto.RegisterResponse
	Peers     []proto.Peer
	Private   keys.Private
	PublicKey string
}

// Do registers this device with the control server and returns the assigned
// identity plus the current peer list. endpoint is the host:port that peers can
// send WireGuard traffic to (may be empty for a control-plane-only join).
func Do(serverURL, token, deviceUUID, hostname, endpoint string) (*Result, error) {
	priv, err := keys.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate node key: %w", err)
	}
	pub := priv.Public().String()

	reqBody, _ := json.Marshal(proto.RegisterRequest{
		Token:      token,
		DeviceUUID: deviceUUID,
		Hostname:   hostname,
		PublicKey:  pub,
		Endpoint:   endpoint,
	})
	client := &http.Client{Timeout: 10 * time.Second}

	var reg proto.RegisterResponse
	if err := postJSON(client, serverURL+"/v1/register", reqBody, &reg); err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	peers, err := FetchPeers(serverURL)
	if err != nil {
		return nil, fmt.Errorf("fetch peers: %w", err)
	}
	return &Result{Register: reg, Peers: peers, Private: priv, PublicKey: pub}, nil
}

// FetchPeers returns the current members of the network.
func FetchPeers(serverURL string) ([]proto.Peer, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	var peers proto.PeersResponse
	if err := getJSON(client, serverURL+"/v1/peers", &peers); err != nil {
		return nil, err
	}
	return peers.Peers, nil
}

// FetchServices returns the current services of the network.
func FetchServices(serverURL string) ([]proto.Service, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	var svcs proto.ServicesResponse
	if err := getJSON(client, serverURL+"/v1/services", &svcs); err != nil {
		return nil, err
	}
	return svcs.Services, nil
}

// PublishService registers (or updates) a service in the network.
func PublishService(serverURL, token string, svc proto.Service) error {
	client := &http.Client{Timeout: 10 * time.Second}
	body, _ := json.Marshal(proto.RegisterServiceRequest{Token: token, Service: svc})
	var out proto.Service
	return postJSON(client, serverURL+"/v1/services", body, &out)
}

func postJSON(c *http.Client, url string, body []byte, out any) error {
	resp, err := c.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func getJSON(c *http.Client, url string, out any) error {
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
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
