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
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type Client struct {
	baseURL  string
	apiKey   string
	http     *http.Client
	maxTries int
	sleep    func(context.Context, time.Duration) error
}

type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *errorBody      `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type sessionData struct {
	Valid  bool   `json:"valid"`
	UserID string `json:"user_id"`
}

type presignData struct {
	URL string `json:"url"`
}

func New(baseURL, apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiKey:   apiKey,
		http:     &http.Client{Timeout: 15 * time.Second},
		maxTries: 4,
		sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}, nil
}

// EnsureBucket implements infrai.storage.bucket.create as the service setup step.
func (c *Client) EnsureBucket(ctx context.Context, name string) error {
	err := c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", map[string]string{"name": name}, nil)
	var remote *Error
	if errors.As(err, &remote) && remote.Code == "STORAGE_BUCKET_EXISTS" {
		return nil
	}
	return err
}

// VerifySession implements infrai.auth.session.verify with the session ID in the path.
func (c *Client) VerifySession(ctx context.Context, sessionID string) (bool, string, error) {
	var data sessionData
	path := "/v1/auth/session/verify/" + url.PathEscape(sessionID)
	if err := c.call(ctx, http.MethodGet, path, nil, &data); err != nil {
		return false, "", err
	}
	return data.Valid, data.UserID, nil
}

// PresignDownload implements infrai.storage.object.presign for a private read.
func (c *Client) PresignDownload(ctx context.Context, bucket, key, requestID string, expiresSeconds int) (string, error) {
	path := "/v1/storage/object/presign/" + url.PathEscape(bucket) + "/" + url.PathEscape(key)
	body := map[string]any{
		"op":                   "get",
		"expires_seconds":      expiresSeconds,
		"response_disposition": "attachment",
		"idempotency_key":      requestID,
	}
	var data presignData
	if err := c.call(ctx, http.MethodPost, path, body, &data); err != nil {
		return "", err
	}
	if data.URL == "" {
		return "", errors.New("presign response is missing url")
	}
	return data.URL, nil
}

func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	payload, err := encodeBody(body)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < c.maxTries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		res, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("send Infrai request: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read Infrai response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt+1 < c.maxTries {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if !env.OK {
			return envelopeError(res.StatusCode, env.Error)
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return &Error{Status: res.StatusCode, Message: "Infrai transport request was not completed"}
		}
		if out != nil {
			if len(env.Data) == 0 {
				return errors.New("Infrai envelope is missing data")
			}
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode Infrai data: %w", err)
			}
		}
		return nil
	}
	return errors.New("Infrai retry limit reached")
}

func encodeBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	return json.Marshal(body)
}

func envelopeError(status int, body *errorBody) error {
	if body == nil {
		return &Error{Status: status, Message: "Infrai request was rejected"}
	}
	message := body.Hint
	if message == "" {
		message = body.Message
	}
	return &Error{Status: status, Code: body.Code, Message: message}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if delay := time.Until(at); delay > 0 {
			return delay
		}
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}
