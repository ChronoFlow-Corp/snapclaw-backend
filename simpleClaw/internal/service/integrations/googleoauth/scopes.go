package googleoauth

import (
	"fmt"
	"slices"
	"strings"
)

var capabilityScopes = map[string][]string{
	CapabilityGmail: {
		"https://www.googleapis.com/auth/gmail.modify",
	},
	CapabilityGoogleCalendar: {
		"https://www.googleapis.com/auth/calendar",
	},
	CapabilitySheets: {
		"https://www.googleapis.com/auth/spreadsheets",
	},
}

var identityScopes = []string{
	"openid",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}

func NormalizeCapabilities(capabilities []string) ([]string, error) {
	if len(capabilities) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(capabilities))
	out := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		normalized := strings.TrimSpace(strings.ToLower(capability))
		if normalized == "" {
			continue
		}

		if _, ok := capabilityScopes[normalized]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnsupportedCapability, normalized)
		}

		if _, ok := seen[normalized]; ok {
			continue
		}

		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	slices.Sort(out)

	return out, nil
}

func BuildScopes(capabilities []string) ([]string, error) {
	normalized, err := NormalizeCapabilities(capabilities)
	if err != nil {
		return nil, err
	}

	scopeSet := make(map[string]struct{}, len(identityScopes)+len(normalized))
	out := append([]string(nil), identityScopes...)
	for _, scope := range identityScopes {
		scopeSet[scope] = struct{}{}
	}

	resourceScopes := make([]string, 0, len(normalized))
	for _, capability := range normalized {
		for _, scope := range capabilityScopes[capability] {
			if _, ok := scopeSet[scope]; ok {
				continue
			}

			scopeSet[scope] = struct{}{}
			resourceScopes = append(resourceScopes, scope)
		}
	}

	slices.Sort(resourceScopes)

	return append(out, resourceScopes...), nil
}

func CapabilitiesFromGrantedScopes(scopes []string) []string {
	scopeSet := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scopeSet[strings.TrimSpace(scope)] = struct{}{}
	}

	out := make([]string, 0, len(capabilityScopes))
	for capability, requiredScopes := range capabilityScopes {
		matched := true
		for _, scope := range requiredScopes {
			if _, ok := scopeSet[scope]; !ok {
				matched = false
				break
			}
		}

		if matched {
			out = append(out, capability)
		}
	}

	slices.Sort(out)

	return out
}
