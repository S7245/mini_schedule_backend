package i18n

import (
	"net/http/httptest"
	"testing"
)

func TestFromRequest(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		acceptLanguage string
		xLanguage      string
		want           Locale
	}{
		{
			name:           "query language wins",
			query:          "?lang=en-US",
			acceptLanguage: "zh-CN",
			want:           LocaleEnUS,
		},
		{
			name:      "explicit language header",
			xLanguage: "zh-Hans",
			want:      LocaleZhCN,
		},
		{
			name:           "accept language",
			acceptLanguage: "fr-FR, en-US;q=0.9, zh-CN;q=0.8",
			want:           LocaleEnUS,
		},
		{
			name:           "unsupported falls back",
			acceptLanguage: "fr-FR",
			want:           DefaultLocale,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/"+tt.query, nil)
			if tt.acceptLanguage != "" {
				req.Header.Set("Accept-Language", tt.acceptLanguage)
			}
			if tt.xLanguage != "" {
				req.Header.Set("X-Language", tt.xLanguage)
			}

			if got := FromRequest(req); got != tt.want {
				t.Errorf("FromRequest(%q, %q) = %q, want %q", tt.query, tt.acceptLanguage, got, tt.want)
			}
		})
	}
}

func TestLocalize(t *testing.T) {
	tests := []struct {
		name     string
		locale   Locale
		key      string
		fallback string
		want     string
	}{
		{
			name:   "english code",
			locale: LocaleEnUS,
			key:    "UNAUTHORIZED",
			want:   "unauthorized",
		},
		{
			name:     "english literal",
			locale:   LocaleEnUS,
			key:      "缺少认证令牌",
			fallback: "缺少认证令牌",
			want:     "missing authentication token",
		},
		{
			name:     "unknown fallback",
			locale:   LocaleEnUS,
			key:      "not_in_catalog",
			fallback: "fallback message",
			want:     "fallback message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Localize(tt.locale, tt.key, tt.fallback); got != tt.want {
				t.Errorf("Localize(%q, %q, %q) = %q, want %q", tt.locale, tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}
