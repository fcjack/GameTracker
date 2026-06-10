package i18n

import (
	"fmt"
	"strings"
)

const (
	LocaleEN   = "en"
	LocalePTBR = "pt-BR"
)

var Supported = []string{LocaleEN, LocalePTBR}

var catalogs = map[string]map[string]string{
	LocaleEN:   enMessages,
	LocalePTBR: ptBRMessages,
}

func Normalize(locale string) string {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "en", "en-us", "en-gb":
		return LocaleEN
	case "pt", "pt-br", "pt_br":
		return LocalePTBR
	default:
		if locale == LocalePTBR {
			return LocalePTBR
		}
		return ""
	}
}

func FromAcceptLanguage(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(strings.Split(part, ";")[0])
		if tag == "" {
			continue
		}
		if loc := Normalize(tag); loc != "" {
			return loc
		}
		if strings.HasPrefix(strings.ToLower(tag), "pt") {
			return LocalePTBR
		}
		if strings.HasPrefix(strings.ToLower(tag), "en") {
			return LocaleEN
		}
	}
	return ""
}

func T(locale, key string, args ...any) string {
	msg := lookup(locale, key)
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

func lookup(locale, key string) string {
	if msgs, ok := catalogs[locale]; ok {
		if msg, ok := msgs[key]; ok {
			return msg
		}
	}
	if msgs, ok := catalogs[LocaleEN]; ok {
		if msg, ok := msgs[key]; ok {
			return msg
		}
	}
	return key
}

func NewTranslator(locale string) func(string, ...any) string {
	if locale == "" {
		locale = LocaleEN
	}
	return func(key string, args ...any) string {
		return T(locale, key, args...)
	}
}
