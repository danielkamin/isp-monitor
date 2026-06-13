package secrets

type Provider interface {
	Get(key string) (string, error)
}
