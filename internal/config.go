package internal

type Config struct {
	LISTEN             string
	PUBLISHER_JWT_KEY  string
	PUBLISHER_JWT_ALG  string
	SUBSCRIBER_JWT_KEY string
	SUBSCRIBER_JWT_ALG string
	CORS_ORIGINS       string
}
