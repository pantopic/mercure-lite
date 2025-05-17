package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/r3labs/sse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	client = &http.Client{Timeout: time.Second}
	parity = os.Getenv("MERCURE_LITE_PARITY")
	target = "http://localhost:8001"

	err error
)

func init() {
	pingPeriod = 10 * time.Millisecond
}

func TestIntegration(t *testing.T) {
	if parity != "" {
		return
	}
	t.Run("base", func(t *testing.T) {
		runIntegrationTest(t, testServer(Config{
			PUBLISHER:  ConfigJWT{JWT_ALG: "RS512", JWT_KEY: pubKeyRS512},
			SUBSCRIBER: ConfigJWT{JWT_ALG: "RS512", JWT_KEY: subKeyRS512},
		}), pubJwtRS512, subJwtRS512, true).Stop()
	})
	t.Run("multi", func(t *testing.T) {
		runIntegrationTest(t, testServer(Config{
			PUBLISHER:  ConfigJWT{JWT_ALG: "RS512", JWT_KEY: pubKeyRS512 + "\n" + subKeyRS512},
			SUBSCRIBER: ConfigJWT{JWT_ALG: "RS512", JWT_KEY: pubKeyRS512 + "\n" + subKeyRS512},
		}), pubJwtRS512, subJwtRS512, true).Stop()
	})
	t.Run("HS256", func(t *testing.T) {
		runIntegrationTest(t, testServer(Config{
			PUBLISHER:  ConfigJWT{JWT_ALG: "HS256", JWT_KEY: pubKeyHS256},
			SUBSCRIBER: ConfigJWT{JWT_ALG: "HS256", JWT_KEY: subKeyHS256},
		}), pubJwtHS256, subJwtHS256, true).Stop()
	})
	t.Run("ES256", func(t *testing.T) {
		runIntegrationTest(t, testServer(Config{
			PUBLISHER:  ConfigJWT{JWT_ALG: "ES256", JWT_KEY: pubKeyES256},
			SUBSCRIBER: ConfigJWT{JWT_ALG: "ES256", JWT_KEY: subKeyES256},
		}), pubJwtES256, subJwtES256, true).Stop()
	})
	t.Run("PS384", func(t *testing.T) {
		runIntegrationTest(t, testServer(Config{
			PUBLISHER:  ConfigJWT{JWT_ALG: "PS384", JWT_KEY: pubKeyPS384},
			SUBSCRIBER: ConfigJWT{JWT_ALG: "PS384", JWT_KEY: subKeyPS384},
		}), pubJwtPS384, subJwtPS384, true).Stop()
	})
}

func TestIntegrationJwks(t *testing.T) {
	if parity != "" {
		return
	}
	s := testServer(Config{
		PUBLISHER:  ConfigJWT{JWKS_URL: "http://example.com/pub"},
		SUBSCRIBER: ConfigJWT{JWKS_URL: "http://example.com/sub"},
	})
	defer s.Stop()
	clk := clock.NewMock()
	s.clock = clk
	s.httpClient.Transport = RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		if r.URL.Path == "/pub" {
			body = `{"keys":[` + pubJwk + `]}`
		} else {
			body = `{"keys":[` + subJwk + `]}`
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})
	runIntegrationTest(t, s, pubJwtRS512, subJwtRS512, true)
	s.httpClient.Transport = RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		if r.URL.Path == "/pub" {
			body = `{"keys":[` + subJwk + `]}`
		} else {
			body = `{"keys":[` + pubJwk + `]}`
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})
	clk.Add(time.Hour)
	runIntegrationTest(t, s, pubJwtRS512, subJwtRS512, false)
}

func TestIntegrationJwksMulti(t *testing.T) {
	if parity != "" {
		return
	}
	s := testServer(Config{
		PUBLISHER: ConfigJWT{
			JWKS_URL: "http://example.com/pub",
		},
		SUBSCRIBER: ConfigJWT{
			JWKS_URL: "http://example.com/sub",
		},
	})
	defer s.Stop()
	clk := clock.NewMock()
	s.clock = clk
	s.httpClient.Transport = RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var body = `{"keys":[` + pubJwk + `,` + subJwk + `]}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})
	runIntegrationTest(t, s, pubJwtRS512, subJwtRS512, true)
	s.httpClient.Transport = RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var body = `{"keys":[` + junkJwk + `]}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})
	clk.Add(time.Hour)
	runIntegrationTest(t, s, pubJwtRS512, subJwtRS512, false)
	s.httpClient.Transport = RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var body = `{"keys":[` + invalidJwk + `]}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})
	clk.Add(time.Hour)
	runIntegrationTest(t, s, pubJwtRS512, subJwtRS512, false)
}

func testServer(cfg Config) *server {
	cfg.LISTEN = ":8001"
	cfg.CORS_ORIGINS = "*"
	return NewServer(cfg)
}

func runIntegrationTest(t *testing.T, s *server, pubJwt, subJwt string, success bool) *server {
	if err := s.Start(t.Context()); err != nil {
		log.Fatal(err)
	}
	ctx1, cancel1 := context.WithCancel(t.Context())
	ctx2, cancel2 := context.WithCancel(t.Context())
	done1 := make(chan bool)
	done2 := make(chan bool)
	subEvents := make(chan *sse.Event)
	sseClientSubs := sse.NewClient(target + "/.well-known/mercure?topic=/.well-known/mercure/subscriptions{/topic}{/subscriber}")
	sseClientSubs.Headers["Authorization"] = "Bearer " + subJwt
	sseClientSubs.SubscribeChanRawWithContext(ctx2, subEvents)
	time.Sleep(10 * time.Millisecond)
	var active bool
	var subEventCount = &atomic.Uint32{}
	go func() {
		for {
			select {
			case e := <-subEvents:
				var sub subscription
				require.Nil(t, json.Unmarshal([]byte(e.Data), &sub))
				active = sub.Active
				if !active {
					cancel2()
				}
				subEventCount.Add(1)
			case <-ctx2.Done():
				close(done2)
				return
			}
		}
	}()
	events := make(chan *sse.Event)
	sseClient := sse.NewClient(target + "/.well-known/mercure?topic=test")
	sseClient.Headers["Authorization"] = "Bearer " + subJwt
	sseClient.SubscribeChanRawWithContext(ctx1, events)
	time.Sleep(10 * time.Millisecond)
	var id, data string
	go func() {
		for {
			select {
			case e := <-events:
				id = string(e.ID)
				data = string(e.Data)
				cancel1()
			case <-ctx1.Done():
				close(done1)
				return
			}
		}
	}()
	req, _ := http.NewRequest("POST", target+"/.well-known/mercure", strings.NewReader(url.Values{
		"data":  {"test-data"},
		"topic": {"test"},
	}.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Authorization", "Bearer "+pubJwt)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Publish error: %v", err)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Publish error: %v", err)
	}
	if success {
		assert.Equal(t, 200, resp.StatusCode)
		<-done1
		assert.Equal(t, id, string(respBody))
		assert.Equal(t, "test-data", data)
		sseClient.Unsubscribe(events)
		<-done2
		assert.Equal(t, false, active)
		assert.EqualValues(t, 2, subEventCount.Load())
	} else {
		assert.Equal(t, 403, resp.StatusCode)
		cancel1()
		cancel2()
	}
	return s
}

func TestApi(t *testing.T) {
	if parity != "" {
		return
	}
	s := testServer(Config{
		PUBLISHER:  ConfigJWT{JWT_ALG: "RS512", JWT_KEY: pubKeyRS512},
		SUBSCRIBER: ConfigJWT{JWT_ALG: "RS512", JWT_KEY: subKeyRS512},
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		log.Fatal(err)
	}
	for range 10 {
		events := make(chan *sse.Event)
		sseClient := sse.NewClient(target + "/.well-known/mercure?topic=test")
		sseClient.Headers["Authorization"] = "Bearer " + subJwtRS512
		sseClient.SubscribeChanRawWithContext(ctx, events)
		go func() {
			defer close(events)
			for {
				select {
				case <-events:
				case <-ctx.Done():
					sseClient.Unsubscribe(events)
					return
				}
			}
		}()
	}
	t.Run("GET", func(t *testing.T) {
		req, _ := http.NewRequest("GET", target+"/.well-known/mercure/subscriptions", nil)
		req.Header.Add("Authorization", "Bearer "+subJwtRS512)
		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("Publish error: %v", err)
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("Publish error: %v", err)
		}
		var data = map[string]any{}
		err = json.Unmarshal(respBody, &data)
		require.Nil(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, 10, len(data["subscriptions"].([]any)))
	})
}

func TestMetrics(t *testing.T) {
	s := testServer(Config{
		PUBLISHER:  ConfigJWT{JWT_ALG: "PS384", JWT_KEY: pubKeyPS384},
		SUBSCRIBER: ConfigJWT{JWT_ALG: "PS384", JWT_KEY: subKeyPS384},
	})
	defer s.Stop()
	s.metrics = NewMetrics(":9090")
	runIntegrationTest(t, s, pubJwtPS384, subJwtPS384, true)
	t.Run("GET", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://localhost:9090/metrics", nil)
		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("Publish error: %v", err)
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("Publish error: %v", err)
		}
		require.Nil(t, err)
		body := string(respBody)
		assert.Contains(t, body, "mercure_lite_connections_active")
		assert.Contains(t, body, "mercure_lite_messages_published 1")
		assert.Contains(t, body, "mercure_lite_messages_sent 3") // Subscribe, message, unsubscribe
		assert.Contains(t, body, "mercure_lite_subscriptions_total 2")
		assert.Contains(t, body, "mercure_lite_connections_terminated 0")
	})
}

// Test JWTs good for 250 years
var subJwtRS512 = `eyJhbGciOiJSUzUxMiIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTc0ODA1MjYzOSwibWVyY3VyZSI6eyJzdWJzY3JpYmUiOlsiLy53ZWxsLWtub3duL21lcmN1cmUvc3Vic2NyaXB0aW9uc3svdG9waWN9ey9zdWJzY3JpYmVyfSIsInRlc3QiXX19.BDTdmm8GkWmCiL3YiPAubyI-Le1wNWGtiXoPYsxFGidfsBC1PbxjEbgarIYsLN7E3POBllsofkJFwD-7CICC-NUt_TWDye4YMy5I75KNYaL2pdn70vm3UrT-zJ-YhKGjp5XkzR9jB4E7PoTj8t6GcEVJKD8V7zCkuLF91Qaxn5VGJ3jdUkK1bR0fzrv4FskTmP3mXQMhO761s9Ktv3Iom_lK23eK-Ta1RKEC7k5nTC29cmyy-vJlNY2bPexJ1iassPgLSRmgLK77MxTZ8jy5vuHcgXSnfYWIQl8M_Qm3p1VudWAgbatKB85M_oI9uks8hCpTI4HU3XcrMpzlmgAJVA`
var subKeyRS512 = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqxkJ3xWZY2pz/WoFi15/
QRrDQUdEb1VBHGy9dHg7Hue1Ss3Ghh3y9Pm+m9dXyqMF9ki7qp6EAcR37s25fo0d
1Vd1TNjkh0mYuiZgc2rrYAArS9V6kssCBseZbW9Z3fBZHqAGdmM8CWAlARPc/kpT
U1I/xZwy38Rb/r8AI1Lsa5dMUxcgMVoADC2GCIihgjUQXsj9ZNNb8wfOzZsWOXD1
xIdSnWVXwkw/08xEkIhMjvRzrPxoK8+453VGnn8UNUyDsLBxR9ln6U3xMpEOV0fO
7FZ9J78iBv9oaHVYl62qczQpksVxMr1uKRVhqIz+3I7NHDpWdHbVaG6w8AR5xkGM
XwIDAQAB
-----END PUBLIC KEY-----`
var subJwk = `{
  "kty": "RSA",
  "use": "sig",
  "alg": "RS512",
  "kid": "0b33c817-fe9d-4ed0-ba03-bc7286c3acc7",
  "n": "qxkJ3xWZY2pz_WoFi15_QRrDQUdEb1VBHGy9dHg7Hue1Ss3Ghh3y9Pm-m9dXyqMF9ki7qp6EAcR37s25fo0d1Vd1TNjkh0mYuiZgc2rrYAArS9V6kssCBseZbW9Z3fBZHqAGdmM8CWAlARPc_kpTU1I_xZwy38Rb_r8AI1Lsa5dMUxcgMVoADC2GCIihgjUQXsj9ZNNb8wfOzZsWOXD1xIdSnWVXwkw_08xEkIhMjvRzrPxoK8-453VGnn8UNUyDsLBxR9ln6U3xMpEOV0fO7FZ9J78iBv9oaHVYl62qczQpksVxMr1uKRVhqIz-3I7NHDpWdHbVaG6w8AR5xkGMXw",
  "e": "AQAB"
}`

var pubJwtRS512 = `eyJhbGciOiJSUzUxMiIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTg0ODA1MjYzOSwibWVyY3VyZSI6eyJwdWJsaXNoIjpbInRlc3QiXX19.H0qakrdoRVW6lqy6S_hWUFegLVPqUdoO_F32IUzAWXzysYo0RkK0FXIwDfd24RL-hPRfj0CibRnz3h6ZjkeRv_GQJK2YSkvZZoy64QTD6vGL5DgcErdqwaY8Ci7X-wdoLpnEyrvjopMLkbYOg9kfwe2aTGsVGNkVGdBrrwZOQMl2yrNTWKiygMVrf0bk91yC0P73SO58PPNHZRwSFnsQqHdUXmnb8-CFqG8nF7xv9ziqkmBiK8DgYoy4n6uQpI28shZKHYO9GDV_6c9v1q9nRyQ5Tw9SwlmZK4HaNMQSKHmKFeZXPK5gILwsEbIVSAK6GJyEGVOmdyHL-vjfxs9JaA`
var pubKeyRS512 = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA14Hlkxs4Uw5J69IsmaMr
VtyHTqBS1Z5ASMEpqs+6TV3CdcsDWp1wuUxzuxexcDCp/qZqZ3QqfKZgOoYDV2Yt
SUbVEA2jUPBFud0mWfwdkYeTztqX4MbK3eENLCfnfaAcIKdpXkrUDIL43DB5VZi8
msp+UgbHsYmPrLvSPznLgjTnuG2WqgO+rQkRAJmy9YZqA1qG4SRrXr2kD7vVA6yZ
t3TaZWsBCy1186w5615k1vmb26Z9EksbSztd5JhS6Nth5EVMi5gl/7NoQiFJF1rS
hTWnWvuQFjqfK1CQwhDN+e8ERPb+agG+nVMI8SYJaiHRsOQFFCCD6dx7HYB75X0X
9QIDAQAB
-----END PUBLIC KEY-----`
var pubJwk = `{
  "kty": "RSA",
  "use": "sig",
  "alg": "RS512",
  "kid": "0b33c817-fe9d-4ed0-ba03-bc7286c3acc7",
  "n": "14Hlkxs4Uw5J69IsmaMrVtyHTqBS1Z5ASMEpqs-6TV3CdcsDWp1wuUxzuxexcDCp_qZqZ3QqfKZgOoYDV2YtSUbVEA2jUPBFud0mWfwdkYeTztqX4MbK3eENLCfnfaAcIKdpXkrUDIL43DB5VZi8msp-UgbHsYmPrLvSPznLgjTnuG2WqgO-rQkRAJmy9YZqA1qG4SRrXr2kD7vVA6yZt3TaZWsBCy1186w5615k1vmb26Z9EksbSztd5JhS6Nth5EVMi5gl_7NoQiFJF1rShTWnWvuQFjqfK1CQwhDN-e8ERPb-agG-nVMI8SYJaiHRsOQFFCCD6dx7HYB75X0X9Q",
  "e": "AQAB"
}`

var junkJwk = `{
  "kty": "RSA",
  "use": "sig",
  "alg": "RS512",
  "kid": "160c8671-5c8c-4435-b91e-84fadfd1abfb",
  "n": "3j-ca362fmuvHCcUgRjcQfvfWLuFHlpq4QIcvE65weHTNLHJgY39mReqzqjXeyE5NDAf55m_Jhou8IE4ESi9tVueC953pmHz8TNtgCO1CYuttcmovdDA3rGWRARtLeSOK5HyEgyyQB3f6nuQmKNlqiQrTXISwkOlOqBNnXZOU2u3a-ZGdoG-rzIGncrJszh58k9ck5-LWkgLm13nHquUswS7fFqEL7YxbiKig_Ts3HJYVP2jhdNdiNTEGd73qY2ULyqM9k1xH_IrdSljQcwSsSdDiNV5rV1LdJbx2c_gCmyEnfbBHwfcHbYWXfOy4AMVeuJMPabM2cVJkkhSis1Yow",
  "e": "AQAB"
}`

var invalidJwk = `{
  "kty": "RSA",
  "use": "sig",
  "alg": "RS512",
  "kid": "160c8671-5c8c-4435-b91e-84fadfd1abfb",
  "n": "3j-ca362fmuvHCcUgRjcQfvfWLuFHlpq4QIcvE65weHTNLHJgY39mReqzqjXeyE5NDAf55m_Jhou8IE4ESi9tVueC953pmHz8TNtgCO1CYuttcmovdDA3rGWRARtLeSOK5HyEgyyQB3f6nuQmKNlqiQrTXISwkOlOqBNnXZOU2u3a-ZGdoG-rzIGncrJszh58k9ck5-LWkgLm13nHquUswS7fFqEL7YxbiKig_Ts3HJYVP2jhdNdiNTEGd73qY2ULyqM9k1xH_IrdSljQcwSsSdDiNV5rV1LdJbx2c_gCmyEnfbBHwfcHbYWXfOy4AMVeuJMPabM2cVJkkhSis1Yow",
  "e": "AQAB",
}`

var subJwtHS256 = `eyJhbGciOiJIUzI1NiIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTc0ODA1MjYzOSwibWVyY3VyZSI6eyJzdWJzY3JpYmUiOlsiLy53ZWxsLWtub3duL21lcmN1cmUvc3Vic2NyaXB0aW9uc3svdG9waWN9ey9zdWJzY3JpYmVyfSIsInRlc3QiXX19.NVI1gYhY9S5EFs30KJyjX6rFsGNOMj9Ko7-AppgErvg`
var subKeyHS256 = `512caae005bf589fb4d7728301205db273d55aa5030a2ab6e2acb2955063b6f1`
var pubJwtHS256 = `eyJhbGciOiJIUzM4NCIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTg0ODA1MjYzOSwibWVyY3VyZSI6eyJwdWJsaXNoIjpbInRlc3QiXX19.MsKRj7Xk6JxVXm7wYGKWavZfn7Xe2izD-209QBs_X5L3TUMnJ0h2UXbmmUHzeUhy`
var pubKeyHS256 = `56500e38ddc0360f0525d7545ba708d1b873aedcc2c5caca1c8077f398b2d409`

var subJwtES256 = `eyJhbGciOiJFUzI1NiIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTc0ODA1MjYzOSwibWVyY3VyZSI6eyJzdWJzY3JpYmUiOlsiLy53ZWxsLWtub3duL21lcmN1cmUvc3Vic2NyaXB0aW9uc3svdG9waWN9ey9zdWJzY3JpYmVyfSIsInRlc3QiXX19.XNnYci4KggJOqQSAsxZZW2dpNtaLbgwgz4iYCAI0PolFkz5icYpp1fGoeD9i65p05kIkznvM58YayDnYIVJeag`
var subKeyES256 = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEboT2CIjLhLJ4973CbWRaQifMkBTN
MJvYIZu6lkxRaC2OnDksfPNtOo6uo/bL21WfTKq1iuFX3E1u79v7rid9kw==
-----END PUBLIC KEY-----`
var pubJwtES256 = `eyJhbGciOiJFUzI1NiIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTg0ODA1MjYzOSwibWVyY3VyZSI6eyJwdWJsaXNoIjpbInRlc3QiXX19.9degmZt7YiMZJ6NBd_wwx3t3WfVGWaVk0iNQRupnW-5fMe8kdOnLQRYeOm2I-B_oOhIIqWh1FbQfjNMmipv_Ow`
var pubKeyES256 = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE65drd/5TBxiKXh7DJ9O5QO7XxoAj
tvEXLn4gaPxc+0fVnVr1gIBRL1dAxZ7CPp7JwnP+WHfc7rIZQAiwisohXw==
-----END PUBLIC KEY-----`

var subJwtPS384 = `eyJhbGciOiJQUzM4NCIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTc0ODA1MjYzOSwibWVyY3VyZSI6eyJzdWJzY3JpYmUiOlsiLy53ZWxsLWtub3duL21lcmN1cmUvc3Vic2NyaXB0aW9uc3svdG9waWN9ey9zdWJzY3JpYmVyfSIsInRlc3QiXX19.e16nzp-so7ONZdnMIwlwGhDP9AHL4MI4DpDrve7q_1zTYDPq-ML2hZq08Zl60DJWfQ3V_kuq9CJl3QWvY40m4kJSKHBs_bqTZHRq3OdAD7lGo5U0RjwM-pQa0TocE5W62i8dmkbkyZ1GKyi1OMhRmF8Pj6sGg_tVURkRazadp1XpU-amxad8sNgqtCL-X0LWCuPjanGb0d6V2kH4_0wwh8Mr5cSCU0ydghiuuMW7nLLxtn0CdRz7vhuQwJ4nDPh7EwLfPvyyRyOBNTlkkWLomBX15pArytn4oJv3IC0ojhIfRq3Ly6P5G_4gxR4IBnn5iD96YygTM_y8r6Em8WU9jQ`
var subKeyPS384 = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAocUHsPngKMeCIQ+xFmhP
rCGbYb35U18HN9gSZBfG3wavSM7oZaSO2fxivcU3kC2vBj8+FhRdGr5ps0ZHvlvo
umeKoWTgs3+/0Ie2AzXtU0UCeW1ActL/lh4nNmhG0tpIPpKnawg1gbjNuRBfQBd/
fCeVmZJ2aKAfIcJCuL/khwgRIf+MQORVXG08vGiGPtcoabTrZkyWnqLNtoS+1uqS
nI9W8Z+xVkIbrX6mwskJi+JVZJ2Y+dE5m+RCUK4stcc03VCoOXnNBZQ8wV49gA67
kFkaxAXHJNCxhsrQFqvqIuXVAaiafq39AyKzs5HQkee5jO29c2nOx8qXFeqxNXlE
TQIDAQAB
-----END PUBLIC KEY-----`
var pubJwtPS384 = `eyJhbGciOiJQUzM4NCIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTg0ODA1MjYzOSwibWVyY3VyZSI6eyJwdWJsaXNoIjpbInRlc3QiXX19.S62Z7EtL0T_jjjXYLYtJPjUKKc-Ku9f6izIxYC0PDQyoS4NSxx2cMtM5U0I5XoPa7JnNjg8iBx5Dsyh82QIRdxV-V2BYdyKtp98IsPgXy12MsIfMFbyTfKS_CgdQ-9IHXFXgGnpwuCrvkJQpY3B4CSpG9h0Bic8Co3AD2Ge7vV21bvA3vCXLEeZCfClJbRO7gA1Ri5nzcZewAgtpnJVGLtiDWUayp2a5PMx5p6XZ6yrjnaNx8UVduIkpxJenzcBFI70aQXOw8bk5WWfvGRbYn4QrSt9xm3G7-RFXo0Jyhcyiom6nWMbDHqlLvDw85aBOrQWjR5smuBLkwQqclIkv1w`
var pubKeyPS384 = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAplOlTxEiRNITSX7jL/t9
JlbxN0xpKvwYKQESDMDwhgSQk3Hvte6VRiWUdUwe/+4PxsCdJ7lj2UJoOn7Xl8xm
bvwma/xW/kZzI2zvjz+3HT4WYLZKEYRyNihf3UsqorvHvhXFaZ46IEbm6ksGs02K
W/fBI1IJx8tSGiaTeIEHMiNAwMIdyKkMCXqIpmM492hbmEqDd/VpnxGW/qViDyrC
kXGmjTIgMm7bP+Lek34IWBJRMmCfu6Tu0o3xqR7q2cXSbIODpY9H1u8iYF2aDB6q
cgFE1w2NrckdFrrTQ03lkcgLMufUgUbFejH5FCHEmeRa+g4pWpFpjxt8gpc1s6Lr
6wIDAQAB
-----END PUBLIC KEY-----`

type RoundTripperFunc func(req *http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
