package internal

import (
	"context"
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
	// "github.com/stretchr/testify/require"
)

var (
	ctx    = context.Background()
	parity = os.Getenv("MERCURE_LITE_PARITY")
	target = "http://localhost:8001"

	err    error
	client = &http.Client{Timeout: time.Second}
)

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(ctx)
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
	events := make(chan *sse.Event)
	sseClient := sse.NewClient(target + "/.well-known/mercure?topic=test")
	sseClient.Headers["Authorization"] = "Bearer " + subJwt
	sseClient.SubscribeChanRaw(events)
	var id, data string
	go func() {
		for {
			select {
			case e := <-events:
				id = string(e.ID)
				data = string(e.Data)
				cancel()
			case <-ctx.Done():
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
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Publish error: %v", err)
	}
	<-ctx.Done()
	assert.Equal(t, id, string(b))
	assert.Equal(t, "test-data", data)
}

var subJwt = `eyJhbGciOiJSUzUxMiIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6MTc0ODA1MjYzOSwibWVyY3VyZSI6eyJzdWJzY3JpYmUiOlsiLy53ZWxsLWtub3duL21lcmN1cmUvc3Vic2NyaXB0aW9uc3svdG9waWN9ey9zdWJzY3JpYmVyfSIsInRlc3QiXX19.QvI85aSkpnLpoyY3KyUsIfDCu7jLvNEf5c7kNdeDzMp838TOhRbC_WTP7e07P3Etf2ruSS3uaa7vBqR6EKcVpS_jXT6bIFvdHTmrZ-1IIORFKTG3FH4BENb5cSyyEMzuF1CJOSAyJnOT_A_pahL5qpZ74RoKSiyZ3HQi_g1f55UX-7VtcbjrPLA9b2RnXoboGB0TbQtRfRAgpOGc8WiK9TJc1VrOJP4pW2IlraOQhadKAUo_hpLWomGzTjHO_ZU63amzN9K0YrL9Yb15VANWOzEAA7C-saNk2N240HTbs_1FZwYhoif2MSWWLrm5k3JRKVizZh8xiye579OSPVObrQ`
var subKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqxkJ3xWZY2pz/WoFi15/
QRrDQUdEb1VBHGy9dHg7Hue1Ss3Ghh3y9Pm+m9dXyqMF9ki7qp6EAcR37s25fo0d
1Vd1TNjkh0mYuiZgc2rrYAArS9V6kssCBseZbW9Z3fBZHqAGdmM8CWAlARPc/kpT
U1I/xZwy38Rb/r8AI1Lsa5dMUxcgMVoADC2GCIihgjUQXsj9ZNNb8wfOzZsWOXD1
xIdSnWVXwkw/08xEkIhMjvRzrPxoK8+453VGnn8UNUyDsLBxR9ln6U3xMpEOV0fO
7FZ9J78iBv9oaHVYl62qczQpksVxMr1uKRVhqIz+3I7NHDpWdHbVaG6w8AR5xkGM
XwIDAQAB
-----END PUBLIC KEY-----`

var pubJwt = `eyJhbGciOiJSUzUxMiIsImNsYXNzaWQiOiJsajF6a3I2emc2c3Uza3U5bW0wdjgifQ.eyJpYXQiOjE3NDcwNTIwMzksImV4cCI6MTg0ODA1MjYzOSwibWVyY3VyZSI6eyJwdWJsaXNoIjpbInRlc3QiXX19.XUDSrwFPG6yaeYkdab-cao7sX7C_2o1OuF4HdmvNC1XjMoaehbVvnpiusfxDg5we2tj7kisEe4fGB7Vpg5RwmHfBrd0oj4BUAru11apPaG4nm2S_Ok0UpXHZtuHFgfj2i4UWUqmRtK4p-SlMlJ9qJg7meOG9DstYdatXNW5T-ZmJdZoZI9xjFBc6wxLg1hjTa9ilKxt6U228SCaXDQ0AI7N0x201NGohKfP-eDV1HkTxlZXyqeoWJvKKgRy44EWXXyEh0GyZArSp7UGstUF_J2cdiweS-M-bAx047LVfN0_TM18lo4Fq9gOPRvJBllBCwQjb7zOIOMM3VwltkELcRA`
var pubKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA14Hlkxs4Uw5J69IsmaMr
VtyHTqBS1Z5ASMEpqs+6TV3CdcsDWp1wuUxzuxexcDCp/qZqZ3QqfKZgOoYDV2Yt
SUbVEA2jUPBFud0mWfwdkYeTztqX4MbK3eENLCfnfaAcIKdpXkrUDIL43DB5VZi8
msp+UgbHsYmPrLvSPznLgjTnuG2WqgO+rQkRAJmy9YZqA1qG4SRrXr2kD7vVA6yZ
t3TaZWsBCy1186w5615k1vmb26Z9EksbSztd5JhS6Nth5EVMi5gl/7NoQiFJF1rS
hTWnWvuQFjqfK1CQwhDN+e8ERPb+agG+nVMI8SYJaiHRsOQFFCCD6dx7HYB75X0X
9QIDAQAB
-----END PUBLIC KEY-----`
