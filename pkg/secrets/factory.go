package secrets

import "os"

func New() (Provider, error) {
	env := os.Getenv("APP_ENV")

	switch env {
	case "production":
		return NewAWSProvider()
	case "development":
		return NewEnvProvider(".env")
	default:
		return NewEnvProvider(".env")
	}
}
