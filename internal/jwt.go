package internal

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/valyala/fasthttp"
)

var (
	algECDSA  = []string{"ES256", "ES384", "ES512"}
	algHMAC   = []string{"HS256", "HS384", "HS512"}
	algRSA    = []string{"RS256", "RS384", "RS512"}
	algRSAPSS = []string{"PS256", "PS384", "PS512"}

	algEdDSA = []string{"EdDSA"}
)

func jwtKeys(alg, key string) (keys []any) {
	if alg == "" {
		return
	}
	if slices.Contains(algHMAC, alg) {
		for k := range bytes.SplitSeq([]byte(key), []byte("\n")) {
			keys = append(keys, k)
		}
		return
	}
	if slices.Contains(algECDSA, alg) ||
		slices.Contains(algRSA, alg) ||
		slices.Contains(algRSAPSS, alg) {
		return x509keys(alg, key)
	}
	if slices.Contains(algEdDSA, alg) {
		log.Println("EdDSA key alg not supported")
		return
	}
	log.Printf("Unrecognized key alg: %s", alg)
	return
}

func x509keys(alg, key string) (keys []any) {
	var rest = []byte(key)
	var block *pem.Block
	var i int
	for len(rest) > 0 {
		i++
		block, rest = pem.Decode(rest)
		if block == nil {
			log.Printf("Unable to decode %s block #%d", alg, i)
			return
		}
		pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			log.Printf("Unable to parse key %s #%d", alg, i)
			return
		}
		switch alg[:2] {
		case "ES":
			keys = append(keys, pubInterface.(*ecdsa.PublicKey))
		case "RS":
			keys = append(keys, pubInterface.(*rsa.PublicKey))
		case "PS":
			keys = append(keys, pubInterface.(*rsa.PublicKey))
		}
	}
	return
}

type tokenClaims struct {
	Mercure struct {
		Publish   []string `json:"publish"`
		Subscribe []string `json:"subscribe"`
	} `json:"mercure"`
	jwt.RegisteredClaims
}

func jwtTokenClaims(ctx *fasthttp.RequestCtx, keys []any) *tokenClaims {
	tokenStr := string(ctx.Request.Header.Peek("Authorization"))
	if parts := strings.Split(tokenStr, " "); len(parts) == 2 {
		tokenStr = parts[1]
	} else {
		tokenStr = string(ctx.Request.Header.Cookie("mercureAuthorization"))
	}
	if tokenStr == "" {
		return nil
	}
	claims := new(tokenClaims)
	var token *jwt.Token
	var err error
	for _, k := range keys {
		token, err = jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
			return k, nil
		})
		if err != nil {
			continue
		}
		if token.Valid {
			break
		}
	}
	if !token.Valid {
		log.Printf("Invalid token: %v", err)
		return nil
	}
	return token.Claims.(*tokenClaims)
}
