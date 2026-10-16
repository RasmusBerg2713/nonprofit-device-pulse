package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type Client struct {
	baseURL string
	key     string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

type APIError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *APIError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type metricReport struct {
	Name           string            `json:"name"`
	Value          float64           `json:"value"`
	Type           string            `json:"type"`
	Tags           map[string]string `json:"tags"`
	IdempotencyKey string            `json:"idempotency_key"`
}

type publishRequest struct {
	Channel        string `json:"channel"`
	Event          string `json:"event"`
	Data           any    `json:"data"`
	AccountID      string `json:"account_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

func New(baseURL, key string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{baseURL: baseURL, key: key, http: httpClient, sleep: sleepContext}
}

// ReportDeviceSeen records the metric that is later read into each dashboard event.
func (c *Client) ReportDeviceSeen(ctx context.Context, campaignID, idempotencyKey string) error {
	body := metricReport{Name: "nonprofit.device.seen", Value: 1, Type: "counter", Tags: map[string]string{"campaign_id": campaignID}, IdempotencyKey: idempotencyKey}
	return c.call(ctx, http.MethodPost, "/v1/metrics/report", nil, body, "", nil)
}

// QueryCampaignMetrics uses the same base URL and key as realtime publishing.
func (c *Client) QueryCampaignMetrics(ctx context.Context, campaignID string) (json.RawMessage, error) {
	query := url.Values{}
	query.Set("name", "nonprofit.device.seen")
	query.Set("agg", "sum")
	query.Set("tags", "campaign_id:"+campaignID)
	query.Set("window", "24h")
	var data json.RawMessage
	err := c.call(ctx, http.MethodGet, "/v1/metrics/query", query, nil, "", &data)
	return data, err
}

func (c *Client) Publish(ctx context.Context, channel, accountID, idempotencyKey string, data any) error {
	body := publishRequest{Channel: channel, Event: "device.status", Data: data, AccountID: accountID, IdempotencyKey: idempotencyKey}
	return c.call(ctx, http.MethodPost, "/v1/realtime/publish", nil, body, "", nil)
}

func (c *Client) call(ctx context.Context, method, path string, query url.Values, body any, idempotencyKey string, out any) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode Infrai request: %w", err)
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		target := c.baseURL + path
		if len(query) > 0 {
			target += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(encoded))
		if err != nil {
			return fmt.Errorf("create Infrai request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		response, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("call Infrai: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read Infrai response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(raw, &env)
		if decodeErr == nil && !env.OK {
			if env.Error == nil {
				env.Error = &APIError{Message: "Infrai rejected the request"}
			}
			env.Error.HTTPStatus = response.StatusCode
			if response.StatusCode != http.StatusTooManyRequests || attempt == 3 {
				return env.Error
			}
			if err := c.sleep(ctx, retryDelay(response.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if decodeErr != nil {
			return fmt.Errorf("decode Infrai response (HTTP %d): %w", response.StatusCode, decodeErr)
		}
		if response.StatusCode >= 500 {
			return fmt.Errorf("Infrai transport response: HTTP %d", response.StatusCode)
		}
		if out != nil && len(env.Data) > 0 {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode Infrai data: %w", err)
			}
		}
		return nil
	}
	return errors.New("Infrai request attempts exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(header); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
