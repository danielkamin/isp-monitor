package config

import "github.com/danielkamin/isp-monitor/pkg/secrets"

func LoadString(p secrets.Provider, key string) (string, error) {
	return p.Get(key)
}
func LoadStringOrDefault(p secrets.Provider, key, fallback string) string {
	configVal, err := p.Get(key)
	if err != nil {
		return fallback
	}
	return configVal
}
