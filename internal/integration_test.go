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

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if parity != "" {
		target = parity
	} else {
		go func() {
			if err := NewServer(Config{
				LISTEN:             ":8001",
				PUBLISHER_JWT_KEY:  pubKeyRS512,
				PUBLISHER_JWT_ALG:  "RS512",
				SUBSCRIBER_JWT_KEY: subKeyRS512,
				SUBSCRIBER_JWT_ALG: "RS512",
				CORS_ORIGINS:       "*",
			}).Start(ctx); err != nil {
				log.Fatal(err)
			}
		}()
		time.Sleep(100 * time.Millisecond)
	}
	runIntegrationTest(t, ctx, pubJwtRS512, subJwtRS512, true)
}

func TestIntegrationMultiKey(t *testing.T) {
	if parity != "" {
		return
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	time.Sleep(100 * time.Millisecond)
	go func() {
		if err := NewServer(Config{
			LISTEN:             ":8001",
			PUBLISHER_JWT_KEY:  pubKeyRS512 + "\n" + subKeyRS512,
			PUBLISHER_JWT_ALG:  "RS512",
			SUBSCRIBER_JWT_KEY: pubKeyRS512 + "\n" + subKeyRS512,
			SUBSCRIBER_JWT_ALG: "RS512",
			CORS_ORIGINS:       "*",
		}).Start(ctx); err != nil {
			log.Fatal(err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	runIntegrationTest(t, ctx, pubJwtRS512, subJwtRS512, true)
}

func TestIntegrationHS256(t *testing.T) {
	if parity != "" {
		return
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	time.Sleep(100 * time.Millisecond)
	go func() {
		if err := NewServer(Config{
			LISTEN:             ":8001",
			PUBLISHER_JWT_KEY:  pubKeyHS256,
			PUBLISHER_JWT_ALG:  "HS256",
			SUBSCRIBER_JWT_KEY: subKeyHS256,
			SUBSCRIBER_JWT_ALG: "HS256",
			CORS_ORIGINS:       "*",
		}).Start(ctx); err != nil {
			log.Fatal(err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	runIntegrationTest(t, ctx, pubJwtHS256, subJwtHS256, true)
}

func TestIntegrationJwks(t *testing.T) {
	if parity != "" {
		return
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	time.Sleep(100 * time.Millisecond)
	s := NewServer(Config{
		LISTEN:              ":8001",
		PUBLISHER_JWKS_URL:  "http://example.com/pub",
		SUBSCRIBER_JWKS_URL: "http://example.com/sub",
		CORS_ORIGINS:        "*",
	})
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
	go func() {
		if err := s.Start(ctx); err != nil {
			log.Fatal(err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	runIntegrationTest(t, ctx, pubJwtRS512, subJwtRS512, true)
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
	time.Sleep(100 * time.Millisecond)
	runIntegrationTest(t, ctx, pubJwtRS512, subJwtRS512, false)
}

func TestIntegrationJwksMulti(t *testing.T) {
	if parity != "" {
		return
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	time.Sleep(100 * time.Millisecond)
	s := NewServer(Config{
		LISTEN:              ":8001",
		PUBLISHER_JWKS_URL:  "http://example.com/pub",
		SUBSCRIBER_JWKS_URL: "http://example.com/sub",
		CORS_ORIGINS:        "*",
	})
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
	go func() {
		if err := s.Start(ctx); err != nil {
			log.Fatal(err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	runIntegrationTest(t, ctx, pubJwtRS512, subJwtRS512, true)
	s.httpClient.Transport = RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var body = `{"keys":[` + junkJwk + `]}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})
	clk.Add(time.Hour)
	time.Sleep(100 * time.Millisecond)
	runIntegrationTest(t, ctx, pubJwtRS512, subJwtRS512, false)
	s.httpClient.Transport = RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var body = `{"keys":[` + invalidJwk + `]}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})
	clk.Add(time.Hour)
	time.Sleep(100 * time.Millisecond)
	runIntegrationTest(t, ctx, pubJwtRS512, subJwtRS512, false)
}

func runIntegrationTest(t *testing.T, ctx context.Context, pubJwt, subJwt string, success bool) {
	oldPing := pingPeriod
	pingPeriod = 100 * time.Millisecond
	defer func() { pingPeriod = oldPing }()
	ctx1, cancel1 := context.WithCancel(ctx)
	ctx2, cancel2 := context.WithCancel(ctx)
	subEvents := make(chan *sse.Event)
	sseClientSubs := sse.NewClient(target + "/.well-known/mercure?topic=/.well-known/mercure/subscriptions{/topic}{/subscriber}")
	sseClientSubs.Headers["Authorization"] = "Bearer " + subJwt
	sseClientSubs.SubscribeChanRawWithContext(ctx2, subEvents)
	var active bool
	var subEventCount int
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
				subEventCount++
			case <-ctx2.Done():
				return
			}
		}
	}()
	time.Sleep(100 * time.Millisecond)
	events := make(chan *sse.Event)
	sseClient := sse.NewClient(target + "/.well-known/mercure?topic=test")
	sseClient.Headers["Authorization"] = "Bearer " + subJwt
	sseClient.SubscribeChanRawWithContext(ctx1, events)
	var id, data string
	go func() {
		for {
			select {
			case e := <-events:
				id = string(e.ID)
				data = string(e.Data)
				cancel1()
			case <-ctx1.Done():
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
		<-ctx1.Done()
		assert.Equal(t, id, string(respBody))
		assert.Equal(t, "test-data", data)
		sseClient.Unsubscribe(events)
		<-ctx2.Done()
		assert.Equal(t, false, active)
		assert.Equal(t, 2, subEventCount)
	} else {
		assert.Equal(t, 403, resp.StatusCode)
		cancel1()
		cancel2()
	}
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

// Test JWT good for 250 years
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

// Test JWTs good for 250 years
var subJwtHS256 = `eyJhbGciOiJIUzI1NiIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTc0ODA1MjYzOSwibWVyY3VyZSI6eyJzdWJzY3JpYmUiOlsiLy53ZWxsLWtub3duL21lcmN1cmUvc3Vic2NyaXB0aW9uc3svdG9waWN9ey9zdWJzY3JpYmVyfSIsInRlc3QiXX19.NVI1gYhY9S5EFs30KJyjX6rFsGNOMj9Ko7-AppgErvg`
var subKeyHS256 = `512caae005bf589fb4d7728301205db273d55aa5030a2ab6e2acb2955063b6f1`
var pubJwtHS256 = `eyJhbGciOiJIUzM4NCIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTg0ODA1MjYzOSwibWVyY3VyZSI6eyJwdWJsaXNoIjpbInRlc3QiXX19.MsKRj7Xk6JxVXm7wYGKWavZfn7Xe2izD-209QBs_X5L3TUMnJ0h2UXbmmUHzeUhy`
var pubKeyHS256 = `56500e38ddc0360f0525d7545ba708d1b873aedcc2c5caca1c8077f398b2d409`

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

type RoundTripperFunc func(req *http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
