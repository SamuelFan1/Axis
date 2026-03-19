package routing

import (
	"regexp"
	"strings"
)

var invalidOriginLabelChars = regexp.MustCompile(`[^a-z0-9-]+`)

func OriginLabelForHostname(hostname string) string {
	label := strings.ToLower(strings.TrimSpace(hostname))
	label = invalidOriginLabelChars.ReplaceAllString(label, "-")
	label = strings.Trim(label, "-")
	label = strings.ReplaceAll(label, "--", "-")
	for strings.Contains(label, "--") {
		label = strings.ReplaceAll(label, "--", "-")
	}
	if label == "" {
		label = "node"
	}
	maxLabelLen := 63 - len("api-origin-")
	if len(label) > maxLabelLen {
		label = label[:maxLabelLen]
	}
	label = strings.Trim(label, "-")
	if label == "" {
		label = "node"
	}
	return "api-origin-" + label
}
