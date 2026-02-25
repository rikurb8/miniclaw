package profile

import (
	"embed"
	"fmt"
	"strings"
)

const defaultProfileName = "default"

//go:embed templates/*.md
var templatesFS embed.FS

func ResolveSystemProfile(provider string) (string, error) {
	templateName := defaultTemplateName(provider)
	if templateName == "" {
		return "", nil
	}

	content, err := templatesFS.ReadFile(templatePath(templateName))
	if err != nil {
		return "", fmt.Errorf("load %s profile template: %w", templateName, err)
	}

	profile := strings.TrimSpace(string(content))
	if profile == "" {
		return "", fmt.Errorf("profile template %q is empty", templateName)
	}

	return profile, nil
}

func templatePath(templateName string) string {
	return "templates/" + strings.TrimSpace(templateName) + ".md"
}
