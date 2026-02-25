package profile

import "strings"

const (
	providerOpenCode = "opencode"
)

func defaultTemplateName(provider string) string {
	if strings.EqualFold(strings.TrimSpace(provider), providerOpenCode) {
		return ""
	}

	return defaultProfileName
}
