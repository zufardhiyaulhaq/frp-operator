package handler

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
)

func Reload(clientCfg models.Config) error {
	if clientCfg.Common.AdminPort == 0 {
		return fmt.Errorf("admin_port shoud be set if you want to use reload feature")
	}

	request, err := http.NewRequest("GET", "http://"+
		clientCfg.Common.AdminAddress+":"+fmt.Sprintf("%d", clientCfg.Common.AdminPort)+"/api/reload", nil)
	if err != nil {
		return err
	}

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(clientCfg.Common.AdminUsername+":"+
		clientCfg.Common.AdminPassword))
	request.Header.Add("Authorization", auth)

	client := http.Client{
		Timeout: 5 * time.Second,
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == 200 {
		return nil
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	return fmt.Errorf("code [%d], %s", response.StatusCode, strings.TrimSpace(string(body)))
}
