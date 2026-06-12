package i18n

import (
	"net/http"
	"strings"

	"golang.org/x/text/language"
)

type Locale string

const (
	LocaleZhCN Locale = "zh-CN"
	LocaleEnUS Locale = "en-US"

	DefaultLocale = LocaleZhCN

	KeySuccess        = "OK"
	KeyInvalidRequest = "INVALID_REQUEST"
)

func FromRequest(r *http.Request) Locale {
	if r == nil {
		return DefaultLocale
	}

	if locale := Normalize(r.URL.Query().Get("lang")); locale != "" {
		return locale
	}
	if locale := Normalize(r.URL.Query().Get("locale")); locale != "" {
		return locale
	}
	if locale := Normalize(r.Header.Get("X-Language")); locale != "" {
		return locale
	}
	if locale := Normalize(r.Header.Get("X-Locale")); locale != "" {
		return locale
	}
	if locale := fromAcceptLanguage(r.Header.Get("Accept-Language")); locale != "" {
		return locale
	}

	return DefaultLocale
}

func Normalize(raw string) Locale {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	tag, err := language.Parse(raw)
	if err == nil {
		base, _ := tag.Base()
		switch base.String() {
		case "zh":
			return LocaleZhCN
		case "en":
			return LocaleEnUS
		}
	}

	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "zh"):
		return LocaleZhCN
	case strings.HasPrefix(lower, "cn"):
		return LocaleZhCN
	case strings.HasPrefix(lower, "en"):
		return LocaleEnUS
	}

	return ""
}

func Localize(locale Locale, key, fallback string) string {
	locale = normalizeLocale(locale)
	if key != "" {
		if message := lookup(locale, key); message != "" {
			return message
		}
		if locale != DefaultLocale {
			if message := lookup(DefaultLocale, key); message != "" {
				return message
			}
		}
	}

	if fallback != "" {
		if message := lookup(locale, fallback); message != "" {
			return message
		}
		if locale != DefaultLocale {
			if message := lookup(DefaultLocale, fallback); message != "" {
				return message
			}
		}
		return fallback
	}

	return key
}

func fromAcceptLanguage(header string) Locale {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}

	tags, _, err := language.ParseAcceptLanguage(header)
	if err == nil {
		for _, tag := range tags {
			if locale := Normalize(tag.String()); locale != "" {
				return locale
			}
		}
	}

	for _, part := range strings.Split(header, ",") {
		value := strings.TrimSpace(strings.Split(part, ";")[0])
		if locale := Normalize(value); locale != "" {
			return locale
		}
	}

	return ""
}

func normalizeLocale(locale Locale) Locale {
	switch locale {
	case LocaleZhCN, LocaleEnUS:
		return locale
	default:
		return DefaultLocale
	}
}

func lookup(locale Locale, key string) string {
	if messages, ok := catalog[locale]; ok {
		return messages[key]
	}
	return ""
}
