package internal

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/lestrrat-go/httpcc"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

func jwksKeys(c *http.Client, url string) (keys []any, maxage time.Duration) {
	if len(url) > 0 {
		resp, err := c.Get(url)
		if err != nil {
			log.Println(err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Printf("JWKS URL returned %d", resp.StatusCode)
			return
		}
		var jwks = map[string][]any{}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println(err)
			return
		}
		if err = json.Unmarshal(body, &jwks); err != nil {
			log.Println(err)
			return
		}
		for _, k := range jwks["keys"] {
			kjson, _ := json.Marshal(k)
			if err := jwk.ParseRawKey(kjson, &k); err != nil {
				log.Println(err)
				return
			}
			keys = append(keys, k)
		}
		maxage = 3600
		directives, err := httpcc.ParseResponse(resp.Header.Get(`Cache-Control`))
		if err == nil {
			val, present := directives.MaxAge()
			if present {
				maxage = time.Duration(max(int(val), 60))
			}
		}
	}
	return
}
