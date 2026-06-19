package secrets

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type EnvProvider struct{}

func NewEnvProvider(path string) (*EnvProvider, error) {
	if err := godotenv.Load(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return &EnvProvider{}, nil

}

func (p *EnvProvider) Get(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("Secret %q, was not found in env", key)
	}
	return val, nil
}
