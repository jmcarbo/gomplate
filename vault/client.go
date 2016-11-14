package vault

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"
)

// Client -
type Client struct {
	Addr *url.URL
	Auth AuthStrategy
	// The cached auth token
	token string
	hc    *http.Client
}

// AuthStrategy -
type AuthStrategy interface {
	fmt.Stringer
	GetToken(addr *url.URL) (string, error)
	Revokable() bool
}

// NewClient - instantiate a new
func NewClient() *Client {
	u := getVaultAddr()
	auth := getAuthStrategy()
	return &Client{u, auth, "", nil}
}

func getVaultAddr() *url.URL {
	vu := os.Getenv("VAULT_ADDR")
	u, err := url.Parse(vu)
	if err != nil {
		log.Fatal("VAULT_ADDR is an unparseable URL!", err)
	}
	return u
}

func getAuthStrategy() AuthStrategy {
	if auth := NewAppIDAuthStrategy(); auth != nil {
		return auth
	}
	if auth := NewTokenAuthStrategy(); auth != nil {
		return auth
	}
	return nil
}

// Login - log in to Vault with the discovered auth backend and save the token
func (c *Client) Login() error {
	token, err := c.Auth.GetToken(c.Addr)
	if err != nil {
		log.Fatal(err)
		return err
	}
	c.token = token
	return nil
}

// RevokeToken - revoke the current auth token - effectively logging out
func (c *Client) RevokeToken() {
	// only do it if the auth strategy supports it!
	if !c.Auth.Revokable() {
		return
	}

	if c.hc == nil {
		c.hc = &http.Client{Timeout: time.Second * 5}
	}

	u := &url.URL{}
	*u = *c.Addr
	u.Path = "/v1/auth/token/revoke-self"
	req, _ := http.NewRequest("POST", u.String(), nil)
	req.Header.Set("X-Vault-Token", c.token)

	res, err := c.hc.Do(req)
	if err != nil {
		log.Println("Error while revoking Vault Token", err)
	}

	if res.StatusCode != 204 {
		log.Printf("Unexpected HTTP status %d on RevokeToken from %s (token was %s)", res.StatusCode, u, c.token)
	}
}

func (c *Client) Read(path string) ([]byte, error) {
	path = normalizeURLPath(path)
	if c.hc == nil {
		c.hc = &http.Client{Timeout: time.Second * 5}
	}

	u := &url.URL{}
	*u = *c.Addr
	u.Path = "/v1" + path
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", c.token)

	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		err = fmt.Errorf("Unexpected HTTP status %d on Read from %s: %s", res.StatusCode, u, body)
		return nil, err
	}

	response := make(map[string]interface{})
	err = json.Unmarshal(body, &response)
	if err != nil {
		log.Println("argh - couldn't decode the response", err)
		return nil, err
	}

	if _, ok := response["data"]; !ok {
		return nil, fmt.Errorf("Unexpected HTTP body on Read for %s: %s", u, body)
	}

	return json.Marshal(response["data"])
}

var rxDupSlashes = regexp.MustCompile(`/{2,}`)

func normalizeURLPath(path string) string {
	if len(path) > 0 {
		path = rxDupSlashes.ReplaceAllString(path, "/")
	}
	return path
}

// ReadResponse -
type ReadResponse struct {
	Data struct {
		Value string `json:"value"`
	} `json:"data"`
}
