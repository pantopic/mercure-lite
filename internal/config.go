package internal

type Config struct {
	LISTEN             string `env:"LISTEN"`
	PUBLISHER_JWT_KEY  string `env:"PUBLISHER_JWT_KEY"`
	PUBLISHER_JWT_ALG  string `env:"PUBLISHER_JWT_ALG"`
	SUBSCRIBER_JWT_KEY string `env:"SUBSCRIBER_JWT_KEY"`
	SUBSCRIBER_JWT_ALG string `env:"SUBSCRIBER_JWT_ALG"`
	CORS_ORIGINS       string `env:"CORS_ORIGINS"`
}
