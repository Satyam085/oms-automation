package oms

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"oms-automtion/config"
	"oms-automtion/models"
)

// ErrRateLimited means OMS kept answering 429 Too Many Requests for longer than
// RateLimitWait. Every other request would be refused too, so a run should stop
// rather than spend the same wait on each outage left.
var ErrRateLimited = errors.New("OMS rate limit (429 Too Many Requests)")

// rateLimitBackoff is the first wait after a 429 that carries no Retry-After;
// it doubles on each further 429.
const rateLimitBackoff = 30 * time.Second

// sleep is swapped out by tests so retries don't take real time.
var sleep = time.Sleep

type Client struct {
	Profile    models.UserProfile
	Token      string
	HTTPClient *http.Client
	Log        *log.Logger
	// RateLimitWait is how long one request may spend waiting out 429s before it
	// fails with ErrRateLimited. Zero means don't wait.
	RateLimitWait time.Duration
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
		Profile:       profile,
		HTTPClient:    &http.Client{Timeout: httpTimeout()},
		Log:           lg,
		RateLimitWait: rateLimitWait(),
	}
}

// httpTimeout is how long to wait for an OMS response. /reason/pending has been
// degrading all day — 104s, 139s, 145s, then 264s — so the default is well clear
// of the trend rather than just clear of the last measurement.
// ponytail: still one timeout for every call. Login and submit answer in seconds,
// so this ceiling only really applies to /pending; give those their own (tighter)
// timeout if a hung submit ever stalls a run for ten minutes.
func httpTimeout() time.Duration {
	if s := os.Getenv("OMS_HTTP_TIMEOUT"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 10 * time.Minute
}

// rateLimitWait reads OMS_RATE_LIMIT_WAIT (seconds). The 15min default outlasts
// the usual rate-limit windows (1 and 15 minutes), so a run waits one out instead
// of dying on it.
func rateLimitWait() time.Duration {
	if s := os.Getenv("OMS_RATE_LIMIT_WAIT"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 15 * time.Minute
}

// do sends req and returns the response status and body. A 429 means OMS is
// rate-limiting us, not that the request is bad: wait as long as it asks
// (Retry-After), or back off if it doesn't say, then send the request again.
// Once RateLimitWait is used up it returns ErrRateLimited.
func (c *Client) do(req *http.Request) (int, []byte, error) {
	var waited time.Duration
	backoff := rateLimitBackoff
	for {
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return 0, nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return 0, nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp.StatusCode, body, nil
		}

		left := c.RateLimitWait - waited
		wait, asked := retryAfter(resp.Header.Get("Retry-After"), time.Now())
		how := "as asked by Retry-After"
		if !asked {
			wait, how = min(backoff, left), "backing off, no Retry-After"
			backoff *= 2
		}
		wait = max(wait, time.Second) // Retry-After: 0 must not become a busy loop
		if wait > left {
			why := fmt.Sprintf("still limited after waiting %s (OMS_RATE_LIMIT_WAIT)", waited)
			if asked {
				why = fmt.Sprintf("it asks for a %s wait (Retry-After), more than the %s left under OMS_RATE_LIMIT_WAIT", wait, left)
			}
			return resp.StatusCode, body, fmt.Errorf("%w on %s %s: %s: %s", ErrRateLimited, req.Method, req.URL.Path, why, body)
		}

		c.Log.Printf("    ⏳ OMS rate limit (429) on %s %s — retrying in %s (%s)", req.Method, req.URL.Path, wait, how)
		sleep(wait)
		waited += wait

		// The first attempt consumed the body; resend it from the top.
		if req.GetBody != nil {
			if req.Body, err = req.GetBody(); err != nil {
				return 0, nil, err
			}
		}
	}
}

// retryAfter parses a Retry-After header, which is either delay-seconds or an
// HTTP-date. ok is false when the header is missing or unparseable.
func retryAfter(header string, now time.Time) (wait time.Duration, ok bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, false
	}
	if s, err := strconv.Atoi(header); err == nil {
		return time.Duration(s) * time.Second, true
	}
	if t, err := http.ParseTime(header); err == nil {
		return t.Sub(now), true
	}
	return 0, false
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

	status, respBody, err := c.do(req)
	if err != nil {
		return fmt.Errorf("login HTTP: %w", err)
	}

	if status != 200 {
		return fmt.Errorf("login returned %d: %s", status, string(respBody))
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
