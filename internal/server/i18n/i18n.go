package i18n

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

var (
	mu          sync.RWMutex
	messages    = map[string]map[string]string{}
	defaultLang = "en"
)

func LoadTranslations(dir string) error {
	mu.Lock()
	defer mu.Unlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		lang := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(dir + "/" + entry.Name())
		if err != nil {
			continue
		}
		var msgs map[string]string
		if err := json.Unmarshal(data, &msgs); err != nil {
			continue
		}
		messages[lang] = msgs
	}

	return nil
}

func T(lang, key string) string {
	mu.RLock()
	defer mu.RUnlock()

	lang = normalizeLang(lang)
	if msgs, ok := messages[lang]; ok {
		if v, ok := msgs[key]; ok {
			return v
		}
	}
	if msgs, ok := messages[defaultLang]; ok {
		if v, ok := msgs[key]; ok {
			return v
		}
	}
	return key
}

func DetectLanguage(acceptLanguage string) string {
	if acceptLanguage == "" {
		return defaultLang
	}
	parts := strings.Split(acceptLanguage, ",")
	if len(parts) > 0 {
		lang := strings.TrimSpace(strings.Split(parts[0], ";")[0])
		return normalizeLang(lang)
	}
	return defaultLang
}

func normalizeLang(lang string) string {
	lang = strings.TrimSpace(strings.ToLower(lang))
	if strings.Contains(lang, "-") {
		lang = strings.Split(lang, "-")[0]
	}
	return lang
}
