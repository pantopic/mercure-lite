package internal

import (
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

	"github.com/r3labs/sse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	ctx    = context.Background()
	client = &http.Client{Timeout: time.Second}
	parity = os.Getenv("MERCURE_LITE_PARITY")
	target = "http://localhost:8001"

	err error
)

func TestIntegration(t *testing.T) {
	pingPeriod = time.Second
	ctx1, cancel1 := context.WithCancel(ctx)
	if parity != "" {
		target = parity
	} else {
		go NewServer(Config{
			LISTEN:             ":8001",
			PUBLISHER_JWT_KEY:  pubKey,
			SUBSCRIBER_JWT_KEY: subKey,
			CORS_ORIGINS:       "*",
		}).Start()
		time.Sleep(100 * time.Millisecond)
	}
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
				log.Printf("%#v", sub)
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
	assert.Equal(t, 200, resp.StatusCode)
	<-ctx1.Done()
	assert.Equal(t, id, string(respBody))
	assert.Equal(t, "test-data", data)
	sseClient.Unsubscribe(events)
	<-ctx2.Done()
	assert.Equal(t, false, active)
	assert.Equal(t, 2, subEventCount)
}

// Test JWT good for 250 years
var subJwt = `eyJhbGciOiJSUzUxMiIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTc0ODA1MjYzOSwibWVyY3VyZSI6eyJzdWJzY3JpYmUiOlsiLy53ZWxsLWtub3duL21lcmN1cmUvc3Vic2NyaXB0aW9uc3svdG9waWN9ey9zdWJzY3JpYmVyfSIsInRlc3QiXX19.BDTdmm8GkWmCiL3YiPAubyI-Le1wNWGtiXoPYsxFGidfsBC1PbxjEbgarIYsLN7E3POBllsofkJFwD-7CICC-NUt_TWDye4YMy5I75KNYaL2pdn70vm3UrT-zJ-YhKGjp5XkzR9jB4E7PoTj8t6GcEVJKD8V7zCkuLF91Qaxn5VGJ3jdUkK1bR0fzrv4FskTmP3mXQMhO761s9Ktv3Iom_lK23eK-Ta1RKEC7k5nTC29cmyy-vJlNY2bPexJ1iassPgLSRmgLK77MxTZ8jy5vuHcgXSnfYWIQl8M_Qm3p1VudWAgbatKB85M_oI9uks8hCpTI4HU3XcrMpzlmgAJVA`
var subKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqxkJ3xWZY2pz/WoFi15/
QRrDQUdEb1VBHGy9dHg7Hue1Ss3Ghh3y9Pm+m9dXyqMF9ki7qp6EAcR37s25fo0d
1Vd1TNjkh0mYuiZgc2rrYAArS9V6kssCBseZbW9Z3fBZHqAGdmM8CWAlARPc/kpT
U1I/xZwy38Rb/r8AI1Lsa5dMUxcgMVoADC2GCIihgjUQXsj9ZNNb8wfOzZsWOXD1
xIdSnWVXwkw/08xEkIhMjvRzrPxoK8+453VGnn8UNUyDsLBxR9ln6U3xMpEOV0fO
7FZ9J78iBv9oaHVYl62qczQpksVxMr1uKRVhqIz+3I7NHDpWdHbVaG6w8AR5xkGM
XwIDAQAB
-----END PUBLIC KEY-----`

// Test JWT good for 250 years
var pubJwt = `eyJhbGciOiJSUzUxMiIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6OTg0ODA1MjYzOSwibWVyY3VyZSI6eyJwdWJsaXNoIjpbInRlc3QiXX19.H0qakrdoRVW6lqy6S_hWUFegLVPqUdoO_F32IUzAWXzysYo0RkK0FXIwDfd24RL-hPRfj0CibRnz3h6ZjkeRv_GQJK2YSkvZZoy64QTD6vGL5DgcErdqwaY8Ci7X-wdoLpnEyrvjopMLkbYOg9kfwe2aTGsVGNkVGdBrrwZOQMl2yrNTWKiygMVrf0bk91yC0P73SO58PPNHZRwSFnsQqHdUXmnb8-CFqG8nF7xv9ziqkmBiK8DgYoy4n6uQpI28shZKHYO9GDV_6c9v1q9nRyQ5Tw9SwlmZK4HaNMQSKHmKFeZXPK5gILwsEbIVSAK6GJyEGVOmdyHL-vjfxs9JaA`
var pubKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA14Hlkxs4Uw5J69IsmaMr
VtyHTqBS1Z5ASMEpqs+6TV3CdcsDWp1wuUxzuxexcDCp/qZqZ3QqfKZgOoYDV2Yt
SUbVEA2jUPBFud0mWfwdkYeTztqX4MbK3eENLCfnfaAcIKdpXkrUDIL43DB5VZi8
msp+UgbHsYmPrLvSPznLgjTnuG2WqgO+rQkRAJmy9YZqA1qG4SRrXr2kD7vVA6yZ
t3TaZWsBCy1186w5615k1vmb26Z9EksbSztd5JhS6Nth5EVMi5gl/7NoQiFJF1rS
hTWnWvuQFjqfK1CQwhDN+e8ERPb+agG+nVMI8SYJaiHRsOQFFCCD6dx7HYB75X0X
9QIDAQAB
-----END PUBLIC KEY-----`
