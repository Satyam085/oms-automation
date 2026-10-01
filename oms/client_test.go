package oms

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"
)

// stubOMS answers request n with statuses[n] (the last one repeats) and records
// each request body. A 429 carries Retry-After when retryAfter is set.
type stubOMS struct {
	mu     sync.Mutex
	bodies []string
}

func newStub(t *testing.T, retryAfter string, statuses ...int) (*stubOMS, *Client, string) {
	s := &stubOMS{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(body))
		status := statuses[min(len(s.bodies), len(statuses))-1]
		s.mu.Unlock()
		if status == http.StatusTooManyRequests {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			// Make the resend use a fresh connection. On a reused one, Go's
			// transport quietly rewinds a drained body itself, which would hide
			// a missing rewind in do.
			w.Header().Set("Connection", "close")
		}
		w.WriteHeader(status)
		io.WriteString(w, http.StatusText(status))
	}))
	t.Cleanup(srv.Close)
	c := &Client{HTTPClient: srv.Client(), Log: log.New(io.Discard, "", 0), RateLimitWait: 15 * time.Minute}
	return s, c, srv.URL
}

func (s *stubOMS) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.bodies)
}

// fakeSleep records waits instead of sleeping through them.
func fakeSleep(t *testing.T) *[]time.Duration {
	var slept []time.Duration
	sleep = func(d time.Duration) {
		if len(slept) == 100 {
			t.Fatalf("still retrying after 100 waits, starting %v", slept[:3])
		}
		slept = append(slept, d)
	}
	t.Cleanup(func() { sleep = time.Sleep })
	return &slept
}

func send(t *testing.T, c *Client, url string) (int, []byte, error) {
	t.Helper()
	req, err := c.NewAPIRequest("POST", url, []byte(`{"offset":0}`))
	if err != nil {
		t.Fatal(err)
	}
	return c.do(req)
}

func TestDoWaitsOutRetryAfterAndResendsBody(t *testing.T) {
	slept := fakeSleep(t)
	stub, c, url := newStub(t, "7", http.StatusTooManyRequests, http.StatusOK)

	status, body, err := send(t, c, url)
	if err != nil || status != http.StatusOK || string(body) != "OK" {
		t.Fatalf("got %d %q %v, want 200 OK", status, body, err)
	}
	if want := []time.Duration{7 * time.Second}; !slices.Equal(*slept, want) {
		t.Errorf("slept %v, want %v (Retry-After)", *slept, want)
	}
	// The resend must carry the body again, not the drained reader.
	if got := stub.requests(); len(got) != 2 || got[1] != `{"offset":0}` {
		t.Errorf("server got bodies %q, want the same body twice", got)
	}
}

func TestDoBacksOffWithoutRetryAfterThenGivesUp(t *testing.T) {
	slept := fakeSleep(t)
	stub, c, url := newStub(t, "", http.StatusTooManyRequests)
	c.RateLimitWait = 2 * time.Minute

	if _, _, err := send(t, c, url); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	// 30s, 60s, then the 30s left of the budget rather than the full 120s.
	if want := []time.Duration{30 * time.Second, time.Minute, 30 * time.Second}; !slices.Equal(*slept, want) {
		t.Errorf("slept %v, want %v", *slept, want)
	}
	if n := len(stub.requests()); n != 4 {
		t.Errorf("sent %d requests, want 4 (first try + 3 retries)", n)
	}
}

func TestDoGivesUpAtOnceWhenAskedToWaitPastBudget(t *testing.T) {
	slept := fakeSleep(t)
	stub, c, url := newStub(t, "3600", http.StatusTooManyRequests)

	if _, _, err := send(t, c, url); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if n := len(stub.requests()); len(*slept) != 0 || n != 1 {
		t.Errorf("slept %v over %d requests, want no wait and one request", *slept, n)
	}
}

func TestDoRetryAfterZeroDoesNotSpin(t *testing.T) {
	slept := fakeSleep(t)
	_, c, url := newStub(t, "0", http.StatusTooManyRequests)
	c.RateLimitWait = 3 * time.Second

	if _, _, err := send(t, c, url); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if want := []time.Duration{time.Second, time.Second, time.Second}; !slices.Equal(*slept, want) {
		t.Errorf("slept %v, want %v", *slept, want)
	}
}

func TestDoPassesOtherStatusesThrough(t *testing.T) {
	slept := fakeSleep(t)
	stub, c, url := newStub(t, "", http.StatusInternalServerError)

	status, _, err := send(t, c, url)
	if n := len(stub.requests()); err != nil || status != http.StatusInternalServerError || n != 1 || len(*slept) != 0 {
		t.Fatalf("got %d %v after %d requests, want one 500 and no retry", status, err, n)
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		header string
		want   time.Duration
		ok     bool
	}{
		{"120", 2 * time.Minute, true},
		{" 5 ", 5 * time.Second, true},
		{now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second, true},
		{"", 0, false},
		{"soon", 0, false},
	}
	for _, c := range cases {
		if got, ok := retryAfter(c.header, now); got != c.want || ok != c.ok {
			t.Errorf("retryAfter(%q) = %v, %v; want %v, %v", c.header, got, ok, c.want, c.ok)
		}
	}
}
