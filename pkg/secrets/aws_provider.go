package secrets

type AWSProvider struct {
}

func NewAWSProvider() (*AWSProvider, error) {

	return &AWSProvider{}, nil

}

func (p *AWSProvider) Get(key string) (string, error) {
	return "", nil
}
