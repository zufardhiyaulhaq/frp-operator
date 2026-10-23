package v1alpha1

import (
	"fmt"
	"slices"
	"strings"
)

type Secret struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type SecretRef struct {
	Secret Secret `json:"secret"`
}

type ConfigMapRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type ConfigMapOrSecretRef struct {
	// +optional
	Secret *Secret `json:"secret,omitempty"`
	// +optional
	ConfigMap *ConfigMapRef `json:"configMap,omitempty"`
}

// LogLevels are the levels frp accepts for log.level.
var LogLevels = []string{"trace", "debug", "info", "warn", "error"}

// LogSpec configures frp's logging. Logs always go to the container's stdout (log.to = "console").
type LogSpec struct {
	// +kubebuilder:validation:Enum=trace;debug;info;warn;error
	// +kubebuilder:default=info
	// +optional
	// Level is the minimum log level: trace, debug, info (default), warn or error.
	Level string `json:"level,omitempty"`
}

// LevelOrDefault returns the configured level, "info" when unset, or an error for a level frp does
// not accept (the CRD enum normally rejects it first; an old CRD or a hand-edited object may not).
func (l *LogSpec) LevelOrDefault() (string, error) {
	if l == nil || l.Level == "" {
		return "info", nil
	}
	if !slices.Contains(LogLevels, l.Level) {
		return "", fmt.Errorf("log.level %q is not one of %s", l.Level, strings.Join(LogLevels, ", "))
	}
	return l.Level, nil
}
