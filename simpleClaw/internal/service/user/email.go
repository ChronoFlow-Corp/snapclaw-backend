package user

import (
	"strings"

	"simpleClaw/internal/entities"
)

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func buildAdminSet(emails []string) map[string]struct{} {
	out := make(map[string]struct{}, len(emails))

	for _, email := range emails {
		email = normalizeEmail(email)
		if email == "" {
			continue
		}

		out[email] = struct{}{}
	}

	return out
}

func (s *Service) roleForEmail(email string) string {
	if _, ok := s.admins[normalizeEmail(email)]; ok {
		return entities.AdminRole
	}

	return entities.UserRole
}
