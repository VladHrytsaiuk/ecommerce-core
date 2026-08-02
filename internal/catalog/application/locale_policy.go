package application

import "strings"

type localePolicy struct{ allowed map[string]struct{} }

func newLocalePolicy(allowedLocales []string) localePolicy {
	allowed := make(map[string]struct{}, len(allowedLocales))
	for _, locale := range allowedLocales {
		if normalized := normalize(locale); normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}
	return localePolicy{allowed: allowed}
}

func (p localePolicy) allows(locale string) bool {
	_, ok := p.allowed[locale]
	return ok
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
