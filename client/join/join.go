// Package join implements the agent-side control-plane join flow.
//
// M1a: generate a node key, register with the control server, and fetch peers.
// No tunnel is established yet — the Wintun adapter and wireguard-go data plane
// arrive in M1b.
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

// Result is the outcome of a successful join.
type Result struct {
	Register  proto.RegisterResponse
	Peers     []proto.Peer
	PublicKey string
}

// Do registers this device with the control server at serverURL and returns the
// assigned identity plus the current peer list.
func Do(serverURL, token, deviceUUID, hostname string) (*Result, error) {
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
	})
	client := &http.Client{Timeout: 10 * time.Second}

	var reg proto.RegisterResponse
	if err := postJSON(client, serverURL+"/v1/register", reqBody, &reg); err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	var peers proto.PeersResponse
	if err := getJSON(client, serverURL+"/v1/peers", &peers); err != nil {
		return nil, fmt.Errorf("fetch peers: %w", err)
	}
	return &Result{Register: reg, Peers: peers.Peers, PublicKey: pub}, nil
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
