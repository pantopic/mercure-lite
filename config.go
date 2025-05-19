package mercurelite

// Config lists the environment variables available.
// All environment variables are prefixed with MERCURE_LITE_
type Config struct {
	// CORS_ORIGINS specifies valid origins for Cross Origin Resource Sharing.
	CORS_ORIGINS string `env:"CORS_ORIGINS" envDefault:"*"`

	// LISTEN specifies the listen address.
	LISTEN string `env:"LISTEN" envDefault:":8001"`

	// PUBLISHER specifies JWT verification config for publishers.
	PUBLISHER ConfigJWT `envPrefix:"PUBLISHER_"`

	// SUBSCRIBER specifies JWT verification config for subscribers.
	SUBSCRIBER ConfigJWT `envPrefix:"SUBSCRIBER_"`

	// METRICS specifies listen interface for prometheus metrics.
	// i.e. http://localhost:9090/metrics
	METRICS string `env:"METRICS" envDefault:":9090"`

	// HUB_COUNT specifies to how many hubs messages should be sharded.
	HUB_COUNT int `env:"HUB_COUNT" envDefault:"16"`

	// DEBUG specifies whether to print invalid JWTs for investigation.
	DEBUG bool `env:"DEBUG" envDefault:"false"`
}

// ConfigJWT specifies the JWT auth configuration for publishers and subscribers.
// Environment variables are prefixed with either PUBLISHER or SUBSCRIBER
// i.e. MERCURE_LITE_PUBLISHER_JWT_KEY, MERCURE_LITE_SUBSCRIBER_JWKS_URL, etc.
type ConfigJWT struct {
	// JWKS_URL specifies the JWKS URL to use for signature verification.
	// See https://datatracker.ietf.org/doc/html/rfc7517
	JWKS_URL string `env:"JWKS_URL" envDefault:""`

	// JWT_KEY specifies the Key to use for JWT signature verification.
	// For asymmetrics algorithms like RSA this is typically a public key in PEM format.
	// Multiple symmetric and asymmetric keys may be specified, newline delimited.
	JWT_KEY string `env:"JWT_KEY" envDefault:"SECRET"`

	// JWT_ALG specifies the Algorithm used by the publisher key.
	// Supported algorithms:
	//   ES256 ES384 ES512 (ECDSA - Elliptic Curve, Asymmetric)
	//   HS256 HS384 HS512 (HMAC, Symmetric)
	//   RS256 RS384 RS512 (RSA, Asymmetric)
	//   PS256 PS384 PS512 (RSA-PSS, Asymmetric)
	JWT_ALG string `env:"JWT_ALG" envDefault:"HS256"`
}
