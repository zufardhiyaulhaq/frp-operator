package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
)

// ProxyStatus is one proxy entry from frpc's GET /api/status.
type ProxyStatus struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Err        string `json:"err"`
	LocalAddr  string `json:"local_addr"`
	Plugin     string `json:"plugin"`
	RemoteAddr string `json:"remote_addr"`
}

// Status calls GET /api/status on the frpc admin API and returns every proxy,
// flattened from frpc's map[type][]proxy and sorted by name.
func Status(clientCfg models.Config) ([]ProxyStatus, error) {
	if clientCfg.Common.AdminPort == 0 {
		return nil, fmt.Errorf("admin_port should be set to read frpc status")
	}

	request, err := http.NewRequest("GET", "http://"+
		clientCfg.Common.AdminAddress+":"+fmt.Sprintf("%d", clientCfg.Common.AdminPort)+"/api/status", nil)
	if err != nil {
		return nil, err
	}

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(clientCfg.Common.AdminUsername+":"+
		clientCfg.Common.AdminPassword))
	request.Header.Add("Authorization", auth)

	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("code [%d], %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var byType map[string][]ProxyStatus
	if err := json.Unmarshal(body, &byType); err != nil {
		return nil, fmt.Errorf("decode frpc status: %w", err)
	}

	proxies := make([]ProxyStatus, 0)
	for _, list := range byType {
		proxies = append(proxies, list...)
	}
	sort.Slice(proxies, func(i, j int) bool { return proxies[i].Name < proxies[j].Name })
	return proxies, nil
}
