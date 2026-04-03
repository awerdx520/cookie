package native

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// CookiePair is a name-value pair returned to the CLI.
type CookiePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// GetCookiesViaSocket connects to the native-host unix socket and queries cookies.
func GetCookiesViaSocket(domain, name string) ([]CookiePair, error) {
	conn, err := net.DialTimeout("unix", SocketPath(), 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to native-host socket: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	req := Request{
		Action: "getCookies",
		Domain: domain,
		Name:   name,
	}
	if err := WriteMessage(conn, req); err != nil {
		return nil, err
	}

	raw, err := ReadMessage(conn)
	if err != nil {
		return nil, err
	}

	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	var cookies []CookiePair
	if err := json.Unmarshal(resp.Data, &cookies); err != nil {
		return nil, fmt.Errorf("parse cookie data: %w", err)
	}
	return cookies, nil
}

// ListDomainsViaSocket connects to the native-host unix socket and lists domains.
func ListDomainsViaSocket() ([]string, error) {
	conn, err := net.DialTimeout("unix", SocketPath(), 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to native-host socket: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	req := Request{Action: "listDomains"}
	if err := WriteMessage(conn, req); err != nil {
		return nil, err
	}

	raw, err := ReadMessage(conn)
	if err != nil {
		return nil, err
	}

	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	var domains []string
	if err := json.Unmarshal(resp.Data, &domains); err != nil {
		return nil, fmt.Errorf("parse domain data: %w", err)
	}
	return domains, nil
}

// ExportCookiesViaSocket tells the native-host to export cookies for a domain to the local file.
func ExportCookiesViaSocket(domain string) error {
	conn, err := net.DialTimeout("unix", SocketPath(), 3*time.Second)
	if err != nil {
		return fmt.Errorf("connect to native-host socket: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	req := Request{Action: "exportCookies", Domain: domain}
	if err := WriteMessage(conn, req); err != nil {
		return err
	}

	raw, err := ReadMessage(conn)
	if err != nil {
		return err
	}

	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}
