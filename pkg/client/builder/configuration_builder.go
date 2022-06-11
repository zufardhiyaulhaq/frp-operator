package builder

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/utils"
)

type ConfigurationBuilder struct {
	Config models.Config
}

func NewConfigurationBuilder() *ConfigurationBuilder {
	return &ConfigurationBuilder{}
}

func (n *ConfigurationBuilder) SetConfig(config models.Config) *ConfigurationBuilder {
	n.Config = config
	return n
}

func (n *ConfigurationBuilder) Build() (string, error) {
	var configurationBuffer bytes.Buffer

	templateEngine, err := template.New("frpc").Parse(utils.CLIENT_TEMPLATE)
	if err != nil {
		return "", err
	}

	err = templateEngine.Execute(&configurationBuffer, n.Config)
	if err != nil {
		return "", err
	}

	var configuration []string
	for _, data := range strings.Split(configurationBuffer.String(), "\n") {
		if len(strings.TrimSpace(data)) != 0 {
			configuration = append(configuration, data)
		}
	}

	return strings.Join(configuration, "\n"), nil
}
