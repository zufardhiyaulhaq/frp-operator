package e2e

import (
	"regexp"
	"strconv"
	"strings"
)

var labelPair = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)="((?:[^"\\]|\\.)*)"`)

// metricValue returns the value of the first sample of metric `name` in a Prometheus text
// exposition whose labels include every entry of `labels`.
func metricValue(body, name string, labels map[string]string) (float64, bool) {
	for _, line := range strings.Split(body, "\n") {
		var labelSet, rest string
		switch {
		case strings.HasPrefix(line, name+"{"):
			end := strings.LastIndex(line, "}")
			if end < 0 {
				continue
			}
			labelSet, rest = line[len(name)+1:end], line[end+1:]
		case strings.HasPrefix(line, name+" "):
			rest = line[len(name):]
		default:
			continue
		}
		if !hasLabels(labelSet, labels) {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		return value, true
	}
	return 0, false
}

func hasLabels(labelSet string, want map[string]string) bool {
	got := map[string]string{}
	for _, m := range labelPair.FindAllStringSubmatch(labelSet, -1) {
		got[m[1]] = m[2]
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}
