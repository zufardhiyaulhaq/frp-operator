package frps

import (
	"bytes"
	"text/template"

	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/utils"
)

// Config is the model rendered into frps.toml.
type Config struct {
	Ports         *Ports
	Token         string
	TLS           *TLSConfig // nil: frps defaults (random certificate, TLS not forced)
	SubdomainHost string
	WebServer     *WebServerConfig // nil: no dashboard, admin API or metrics
	LogLevel      string           // "" renders "info"
}

type TLSConfig struct {
	Force                            bool
	CertFile, KeyFile, TrustedCAFile string // "" leaves the key out
}

type WebServerConfig struct {
	Port              int32
	User, Password    string
	CertFile, KeyFile string // HTTPS when set
	Prometheus        bool
}

// Render renders frps.toml.
func Render(c Config) (string, error) {
	tmpl, err := template.New("frps").Funcs(utils.TemplateFuncs).Parse(FRPS_TEMPLATE)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, c); err != nil {
		return "", err
	}
	return buf.String(), nil
}
