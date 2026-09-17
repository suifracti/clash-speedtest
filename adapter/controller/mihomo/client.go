package mihomo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/controller"
)

// Config configures the Mihomo External Controller REST client.
type Config struct {
	// Endpoint is the base URL of the External Controller, e.g. "http://127.0.0.1:9090" or "https://remote:9090".
	Endpoint string `json:"endpoint"`
	// Secret is the optional Bearer token configured in Mihomo external-controller-secret.
	// Never log or serialize this in plaintext in UI/DTOs.
	Secret string `json:"secret,omitempty"`
	// Timeout specifies the HTTP request timeout. Defaults to 5s.
	Timeout time.Duration `json:"timeout,omitempty"`
	// AllowRemote explicitly permits connecting to a non-loopback external controller endpoint.
	// By default, only localhost/loopback connections are allowed for local security.
	AllowRemote bool `json:"allow_remote"`
	// AllowInsecurePlaintextRemote explicitly allows unencrypted HTTP over a remote non-loopback network.
	// High security risk (secret and control commands sent in cleartext); strongly discouraged.
	AllowInsecurePlaintextRemote bool `json:"allow_insecure_plaintext_remote"`
}

// MaskedSecret returns a safe, redacted representation of the secret.
func (c Config) MaskedSecret() string {
	if c.Secret == "" {
		return ""
	}
	if len(c.Secret) <= 4 {
		return "****"
	}
	return c.Secret[:2] + "****" + c.Secret[len(c.Secret)-2:]
}

// Client implements controller.Controller via Mihomo's official External Controller REST API.
type Client struct {
	endpoint string
	secret   string
	client   *http.Client
}

// NewClient creates a new Mihomo External Controller REST client with loopback and transport security enforcement.
func NewClient(cfg Config) (*Client, error) {
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:9090"
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}

	isLoopback := isLoopbackEndpoint(endpoint)
	isHTTPS := strings.HasPrefix(strings.ToLower(endpoint), "https://")

	// Security Enforcement:
	if !isLoopback {
		// Non-loopback remote connection check
		if !cfg.AllowRemote {
			return nil, fmt.Errorf("external controller endpoint %q is not localhost/loopback; by default remote connections are disabled for security (set AllowRemote=true to enable)", endpoint)
		}
		// Non-loopback remote connections must use HTTPS / Mihomo external-controller-tls unless explicit high-risk opt-in
		if !isHTTPS && !cfg.AllowInsecurePlaintextRemote {
			return nil, fmt.Errorf("remote external controller endpoint %q uses insecure plaintext HTTP; remote connections require HTTPS (Mihomo external-controller-tls) for security, or explicit AllowInsecurePlaintextRemote=true opt-in", endpoint)
		}
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	return &Client{
		endpoint: endpoint,
		secret:   cfg.Secret,
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// GetVersion returns Mihomo Core version information.
func (c *Client) GetVersion(ctx context.Context) (controller.VersionInfo, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/version", nil)
	if err != nil {
		return controller.VersionInfo{}, err
	}

	var raw struct {
		Version string `json:"version"`
		Premium bool   `json:"premium"`
	}
	if err := c.doJSON(req, &raw); err != nil {
		return controller.VersionInfo{}, fmt.Errorf("mihomo /version: %w", err)
	}

	return controller.VersionInfo{
		Version:  raw.Version,
		Premium:  raw.Premium,
		CoreType: "mihomo",
	}, nil
}

// GetCapabilities returns supported features of the Mihomo external controller.
func (c *Client) GetCapabilities(ctx context.Context) (controller.Capabilities, error) {
	return controller.Capabilities{
		CanSelectNode:       true,
		CanTestDelay:        true,
		CanStreamLogs:       true,
		CanListConnections:  true,
		SupportsMemoryStats: true,
	}, nil
}

// ListGroups returns all proxy groups (Selector, URLTest, Fallback, etc.)
func (c *Client) ListGroups(ctx context.Context) ([]controller.Group, error) {
	proxies, err := c.fetchProxiesMap(ctx)
	if err != nil {
		return nil, err
	}

	var groups []controller.Group
	for _, raw := range proxies {
		groupType := strings.ToLower(raw.Type)
		if isGroupType(groupType) {
			groups = append(groups, controller.Group{
				Name: raw.Name,
				Type: raw.Type,
				Now:  raw.Now,
				All:  raw.All,
			})
		}
	}
	return groups, nil
}

// ListNodes returns all candidate nodes belonging to a specific group, or all nodes if group is empty.
func (c *Client) ListNodes(ctx context.Context, group string) ([]controller.Node, error) {
	proxies, err := c.fetchProxiesMap(ctx)
	if err != nil {
		return nil, err
	}

	if group != "" {
		g, ok := proxies[group]
		if !ok {
			return nil, fmt.Errorf("group %q not found in core", group)
		}
		var nodes []controller.Node
		for _, nodeName := range g.All {
			if nodeRaw, exists := proxies[nodeName]; exists {
				nodes = append(nodes, rawToNode(nodeRaw))
			} else {
				nodes = append(nodes, controller.Node{Name: nodeName})
			}
		}
		return nodes, nil
	}

	var nodes []controller.Node
	for _, raw := range proxies {
		if !isGroupType(strings.ToLower(raw.Type)) && !isBuiltInSpecial(raw.Name) {
			nodes = append(nodes, rawToNode(raw))
		}
	}
	return nodes, nil
}

// GetCurrentSelection returns the currently selected node for the specified group.
func (c *Client) GetCurrentSelection(ctx context.Context, group string) (string, error) {
	raw, err := c.getGroupRaw(ctx, group)
	if err != nil {
		return "", err
	}
	return raw.Now, nil
}

// SelectNode updates the active proxy selection for a Selector group in the external core.
// Crucial Safety Boundaries:
// 1. Only Selector groups are permitted to be updated; URLTest, Fallback, and LoadBalance groups
//    are read-only to preserve core native automatic selection semantics.
// 2. Updating the selection instructs Mihomo to route new connections via nodeName; existing active user
//    connections are preserved by default (no DELETE /connections call).
func (c *Client) SelectNode(ctx context.Context, group string, nodeName string) error {
	groupInfo, err := c.getGroupRaw(ctx, group)
	if err != nil {
		return fmt.Errorf("verify group %q: %w", group, err)
	}

	if strings.ToLower(groupInfo.Type) != "selector" {
		return fmt.Errorf("group %q has type %q; auto-switching only officially manages Selector groups (URLTest, Fallback, and LoadBalance groups are read-only to preserve native semantics)", group, groupInfo.Type)
	}

	payload, err := json.Marshal(map[string]string{
		"name": nodeName,
	})
	if err != nil {
		return err
	}

	escapedGroup := url.PathEscape(group)
	req, err := c.newRequest(ctx, http.MethodPut, "/proxies/"+escapedGroup, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("select node %q in group %q: %w", nodeName, group, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("select node failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// TestDelay invokes Mihomo's built-in delay test for a single proxy.
func (c *Client) TestDelay(ctx context.Context, proxyName string, testURL string, timeout time.Duration) (time.Duration, error) {
	if testURL == "" {
		testURL = "http://www.gstatic.com/generate_204"
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	q := url.Values{}
	q.Set("url", testURL)
	q.Set("timeout", fmt.Sprintf("%d", timeout.Milliseconds()))

	escapedProxy := url.PathEscape(proxyName)
	path := fmt.Sprintf("/proxies/%s/delay?%s", escapedProxy, q.Encode())
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return 0, err
	}

	var raw struct {
		Delay int    `json:"delay"`
		Msg   string `json:"message,omitempty"`
	}
	if err := c.doJSON(req, &raw); err != nil {
		return 0, fmt.Errorf("test delay for %q: %w", proxyName, err)
	}

	if raw.Delay <= 0 && raw.Msg != "" {
		return 0, fmt.Errorf("delay test failed: %s", raw.Msg)
	}
	return time.Duration(raw.Delay) * time.Millisecond, nil
}

// Internal helpers

type rawProxy struct {
	Name    string                  `json:"name"`
	Type    string                  `json:"type"`
	UDP     bool                    `json:"udp"`
	Now     string                  `json:"now"`
	All     []string                `json:"all"`
	History []controller.DelayRecord `json:"history"`
}

func (c *Client) getGroupRaw(ctx context.Context, group string) (*rawProxy, error) {
	escapedGroup := url.PathEscape(group)
	req, err := c.newRequest(ctx, http.MethodGet, "/proxies/"+escapedGroup, nil)
	if err != nil {
		return nil, err
	}

	var raw rawProxy
	if err := c.doJSON(req, &raw); err != nil {
		return nil, fmt.Errorf("get group %q: %w", group, err)
	}
	return &raw, nil
}

func (c *Client) fetchProxiesMap(ctx context.Context) (map[string]rawProxy, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/proxies", nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Proxies map[string]rawProxy `json:"proxies"`
	}
	if err := c.doJSON(req, &raw); err != nil {
		return nil, fmt.Errorf("fetch /proxies: %w", err)
	}
	return raw.Proxies, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	fullURL := c.endpoint + path
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, err
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	return req, nil
}

func (c *Client) doJSON(req *http.Request, target interface{}) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func isLoopbackEndpoint(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "localhost.localdomain" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isGroupType(t string) bool {
	switch strings.ToLower(t) {
	case "selector", "urltest", "fallback", "loadbalance", "relay":
		return true
	default:
		return false
	}
}

func isBuiltInSpecial(name string) bool {
	switch strings.ToUpper(name) {
	case "DIRECT", "REJECT", "GLOBAL", "COMPATIBLE", "PASS":
		return true
	default:
		return false
	}
}

func rawToNode(raw rawProxy) controller.Node {
	return controller.Node{
		Name:    raw.Name,
		Type:    raw.Type,
		UDP:     raw.UDP,
		History: raw.History,
	}
}
