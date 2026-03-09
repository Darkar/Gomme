package i18n

import (
	"encoding/json"
	"log"
	"os"
	"strings"
)

var translations = map[string]map[string]string{}

// Load reads all .json files from localesDir and stores translations.
func Load(localesDir string) {
	entries, err := os.ReadDir(localesDir)
	if err != nil {
		log.Printf("i18n: cannot read locales dir %s: %v", localesDir, err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		lang := strings.TrimSuffix(e.Name(), ".json")
		data, err := os.ReadFile(localesDir + "/" + e.Name())
		if err != nil {
			log.Printf("i18n: cannot read %s: %v", e.Name(), err)
			continue
		}
		m := map[string]string{}
		if err := json.Unmarshal(data, &m); err != nil {
			log.Printf("i18n: cannot parse %s: %v", e.Name(), err)
			continue
		}
		translations[lang] = m
		log.Printf("i18n: %d keys loaded for lang=%s", len(m), lang)
	}
}

// T returns the translation for key in lang, falling back to "en", then the key itself.
func T(lang, key string) string {
	if m, ok := translations[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := translations["en"]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

// GetTranslator returns a function that translates keys for a given language.
func GetTranslator(lang string) func(string) string {
	return func(key string) string { return T(lang, key) }
}

// Detect parses the Accept-Language header and returns "fr" or "en" (default).
func Detect(acceptLang string) string {
	for _, part := range strings.Split(acceptLang, ",") {
		tag := strings.TrimSpace(part)
		if idx := strings.Index(tag, ";"); idx >= 0 {
			tag = strings.TrimSpace(tag[:idx])
		}
		tag = strings.ToLower(tag)
		if strings.HasPrefix(tag, "fr") {
			return "fr"
		}
		if strings.HasPrefix(tag, "en") {
			return "en"
		}
	}
	return "en"
}
