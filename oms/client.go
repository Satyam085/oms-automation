package oms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"oms-automtion/config"
	"oms-automtion/models"
)

type Client struct {
	Profile    models.UserProfile
	Token      string
	HTTPClient *http.Client
	Log        *log.Logger
}

// NewClient builds a client for one profile. lg receives progress lines (pass the
// run's logger so they reach the web UI); nil falls back to the default logger.
func NewClient(profile models.UserProfile, lg *log.Logger) *Client {
	if lg == nil {
		lg = log.Default()
	}
	if profile.EmpNo == "" {
		profile.CompanyName = config.Creds.CompanyName
		profile.EmpNo = config.Creds.EmpNo
		profile.Password = config.Creds.Password
		profile.AppName = config.Creds.AppName
	}
	return &Client{
		Profile:    profile,
		HTTPClient: &http.Client{Timeout: httpTimeout()},
		Log:        lg,
	}
}

// httpTimeout is how long to wait for an OMS response. /reason/pending regularly
// takes minutes when the server is loaded, so the default is generous.
// ponytail: single timeout for every call; split per-endpoint only if one needs it.
func httpTimeout() time.Duration {
	if s := os.Getenv("OMS_HTTP_TIMEOUT"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 3 * time.Minute
}

// Login authenticates and sets the client's Token fields.
func (c *Client) Login() error {
	payload := models.LoginRequest{
		CompanyName: c.Profile.CompanyName,
		EmpNo:       c.Profile.EmpNo,
		Password:    c.Profile.Password,
		AppName:     c.Profile.AppName,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal login: %w", err)
	}

	req, err := http.NewRequest("POST", config.BaseURL+"/auth/login", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}
	// Login sends "bearer null" initially — no valid token yet
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Authorization", "bearer null")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", "https://smartoms.geourja.com/")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("login HTTP: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("login returned %d: %s", resp.StatusCode, string(respBody))
	}

	var loginResp models.LoginResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return fmt.Errorf("unmarshal login response: %w", err)
	}

	if loginResp.User.AuthToken == "" {
		return fmt.Errorf("login failed: auth_token missing")
	}

	c.Token = loginResp.User.AuthToken
	c.Log.Printf("  ✓ Logged in as empNo=%s", c.Profile.EmpNo)
	return nil
}

// NewAPIRequest builds an http.Request with all required OMS headers
func (c *Client) NewAPIRequest(method, url string, body []byte) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "bearer "+c.Token)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", "https://smartoms.geourja.com/")
	return req, nil
}
