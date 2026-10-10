package httpc

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
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultUserAgent      = "wallet-sdk/1.0 (+https://github.com/kernelflowlabs/wallet-sdk)"
	defaultTimeout        = 60 * time.Second
	defaultRateLimitQPS   = 200
	defaultRateLimitBurst = 500
	errorBodyReadLimit    = 4096
)

// HTTPStatusError preserves non-2xx response metadata so callers can make
// decisions based on the status without parsing an error string. Preview and
// URL are intended for diagnostics only and should not be shown to end users.
type HTTPStatusError struct {
	StatusCode        int
	RetryAfterSeconds int
	URL               string
	Preview           string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "unexpected HTTP status"
	}
	if e.URL == "" {
		return fmt.Sprintf("unexpected status=%d", e.StatusCode)
	}
	return fmt.Sprintf("unexpected status=%d, url=%s", e.StatusCode, safeURL(e.URL))
}

type ResponseTooLargeError struct {
	MaxBytes      int64
	ContentLength int64
	URL           string
}

func (e *ResponseTooLargeError) Error() string {
	if e == nil {
		return "response exceeds configured size limit"
	}
	if e.ContentLength >= 0 {
		return fmt.Sprintf("response size %d exceeds limit %d for %s", e.ContentLength, e.MaxBytes, safeURL(e.URL))
	}
	return fmt.Sprintf("response exceeds limit %d for %s", e.MaxBytes, safeURL(e.URL))
}

type Request struct {
	stateLock        sync.RWMutex
	baseUrl          string
	httpClient       *http.Client
	limiter          *rate.Limiter
	maxResponseBytes int64
	headers          map[string]string
	headerLock       sync.RWMutex
}

func NewRequest(baseUrl string, headers map[string]string) *Request {
	request, _ := newRequest(baseUrl, headers)
	return request
}

func NewRequestWithOptions(baseUrl string, headers map[string]string, options ...Option) (*Request, error) {
	return newRequest(baseUrl, headers, options...)
}

func newRequest(baseUrl string, headers map[string]string, optionList ...Option) (*Request, error) {
	options := requestOptions{
		rateLimitEnabled: true,
		rateLimitQPS:     defaultRateLimitQPS,
		rateLimitBurst:   defaultRateLimitBurst,
	}
	for _, option := range optionList {
		if option == nil {
			return nil, fmt.Errorf("http client option must not be nil")
		}
		if err := option(&options); err != nil {
			return nil, err
		}
	}

	client := options.httpClient
	if client == nil {
		client = defaultHTTPClient()
	} else {
		copy := *client
		client = &copy
	}
	if options.timeoutSet {
		client.Timeout = options.timeout
	}

	var limiter *rate.Limiter
	if options.rateLimitEnabled {
		limiter = rate.NewLimiter(rate.Limit(options.rateLimitQPS), options.rateLimitBurst)
	}

	return &Request{
		baseUrl:          baseUrl,
		headers:          cloneHeaders(headers),
		httpClient:       client,
		limiter:          limiter,
		maxResponseBytes: options.maxResponseBytes,
	}, nil
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: defaultTimeout,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     60 * time.Second,
			DisableKeepAlives:   false,
		},
	}
}

func cloneHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	copy := make(map[string]string, len(headers))
	for key, value := range headers {
		copy[key] = value
	}
	return copy
}

func (r *Request) SetBaseUrl(baseUrl string) {
	r.stateLock.Lock()
	r.baseUrl = baseUrl
	r.stateLock.Unlock()
}

func (r *Request) SetRateLimit(qps float64, burst int) {
	if burst <= 0 {
		burst = 1
	}
	r.stateLock.Lock()
	r.limiter = rate.NewLimiter(rate.Limit(qps), burst)
	r.stateLock.Unlock()
}

func (r *Request) SetHeader(k, v string) {
	r.headerLock.Lock()
	defer r.headerLock.Unlock()
	if r.headers == nil {
		r.headers = make(map[string]string)
	}
	r.headers[k] = v
}

func (r *Request) Get(ctx context.Context, result interface{}, path string, query url.Values) error {
	uri, err := r.buildURL(path, query)
	if err != nil {
		return err
	}
	return r.Execute(ctx, http.MethodGet, uri, nil, result)
}

func (r *Request) GetWithHeaders(ctx context.Context, result interface{}, path string, query url.Values, headers map[string]string) error {
	uri, err := r.buildURL(path, query)
	if err != nil {
		return err
	}
	return r.execute(ctx, http.MethodGet, uri, nil, result, headers)
}

func (r *Request) GetRaw(ctx context.Context, result *bytes.Buffer, path string, query url.Values) error {
	uri, err := r.buildURL(path, query)
	if err != nil {
		return err
	}
	return r.ExecuteRaw(ctx, http.MethodGet, uri, nil, result)
}

func (r *Request) Post(ctx context.Context, result interface{}, path string, body interface{}) error {
	buf, err := GetJsonBody(body)
	if err != nil {
		return fmt.Errorf("failed to GetBody,err=%v", err)
	}
	uri, err := r.buildURL(path, nil)
	if err != nil {
		return err
	}
	return r.Execute(ctx, http.MethodPost, uri, buf, result)
}

func (r *Request) PostWithXWWWFormUrlencoded(ctx context.Context, result interface{}, path string, body interface{}) error {
	var buf io.Reader
	if params, ok := body.(url.Values); ok {
		buf = strings.NewReader(params.Encode())
	}
	uri, err := r.buildURL(path, nil)
	if err != nil {
		return err
	}
	return r.Execute(ctx, http.MethodPost, uri, buf, result)
}

func (r *Request) PostWithOutEncoded(ctx context.Context, result interface{}, path string, body interface{}) error {
	b, ok := body.([]byte)
	if !ok {
		return fmt.Errorf("body must be []byte, got %T", body)
	}
	uri, err := r.buildURL(path, nil)
	if err != nil {
		return err
	}
	return r.Execute(ctx, http.MethodPost, uri, bytes.NewBuffer(b), result)
}

func (r *Request) PostWithPlain(ctx context.Context, result interface{}, path string, body io.Reader) error {
	uri, err := r.buildURL(path, nil)
	if err != nil {
		return err
	}
	return r.Execute(ctx, http.MethodPost, uri, body, result)
}

func (r *Request) Delete(ctx context.Context, result interface{}, path string) error {
	uri, err := r.buildURL(path, nil)
	if err != nil {
		return err
	}
	return r.Execute(ctx, http.MethodDelete, uri, nil, result)
}

func (r *Request) Patch(ctx context.Context, result interface{}, path string, body interface{}) error {
	buf, err := GetJsonBody(body)
	if err != nil {
		return fmt.Errorf("failed to GetBody,err=%v", err)
	}
	uri, err := r.buildURL(path, nil)
	if err != nil {
		return err
	}
	return r.Execute(ctx, http.MethodPatch, uri, buf, result)
}

func (r *Request) GetBase(path string) string {
	uri, err := r.buildURL(path, nil)
	if err != nil {
		return ""
	}
	return uri
}

func (r *Request) buildURL(path string, query url.Values) (string, error) {
	r.stateLock.RLock()
	baseUrl := r.baseUrl
	r.stateLock.RUnlock()

	parsed, err := url.Parse(baseUrl)
	if err != nil {
		return "", fmt.Errorf("invalid base URL")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("base URL must be absolute")
	}
	if path != "" {
		parsed = parsed.JoinPath(strings.TrimLeft(path, "/"))
	}
	if query != nil {
		values := parsed.Query()
		for key, entries := range query {
			values.Del(key)
			for _, value := range entries {
				values.Add(key, value)
			}
		}
		parsed.RawQuery = values.Encode()
	}
	return parsed.String(), nil
}

func GetJsonBody(body interface{}) (buf io.ReadWriter, err error) {
	if body != nil {
		buf = new(bytes.Buffer)
		err = json.NewEncoder(buf).Encode(body)
	}
	return
}

func (r *Request) Execute(ctx context.Context, method string, requestURL string, body io.Reader, result interface{}) error {
	return r.execute(ctx, method, requestURL, body, result, nil)
}

func (r *Request) execute(ctx context.Context, method string, requestURL string, body io.Reader, result interface{}, headers map[string]string) error {
	res, maxResponseBytes, err := r.executeRequest(ctx, method, requestURL, body, headers)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, responseErrorReadLimit(maxResponseBytes)))
		return newHTTPStatusError(res, requestURL)
	}

	b, err := readResponseBody(res.Body, maxResponseBytes, res.ContentLength, requestURL)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	if len(bytes.TrimSpace(b)) == 0 {
		if res.StatusCode == http.StatusNoContent || res.StatusCode == http.StatusResetContent {
			return nil
		}
		return fmt.Errorf("empty response body, status=%d, url=%s", res.StatusCode, safeURL(requestURL))
	}
	if err := json.Unmarshal(b, result); err != nil {
		return fmt.Errorf("unmarshal failed, status=%d, url=%s: %w", res.StatusCode, safeURL(requestURL), err)
	}
	return nil
}

func (r *Request) ExecuteRaw(ctx context.Context, method string, requestURL string, body io.Reader, result *bytes.Buffer) error {
	res, maxResponseBytes, err := r.executeRequest(ctx, method, requestURL, body, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		if result != nil {
			_, _ = io.Copy(result, io.LimitReader(res.Body, responseErrorReadLimit(maxResponseBytes)))
		} else {
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, responseErrorReadLimit(maxResponseBytes)))
		}
		return newHTTPStatusError(res, requestURL)
	}

	b, err := readResponseBody(res.Body, maxResponseBytes, res.ContentLength, requestURL)
	if err != nil {
		return err
	}
	if result != nil {
		_, err = result.Write(b)
	}
	return err
}

func (r *Request) executeRequest(ctx context.Context, method string, requestURL string, body io.Reader, headers map[string]string) (*http.Response, int64, error) {
	r.stateLock.RLock()
	limiter := r.limiter
	client := r.httpClient
	maxResponseBytes := r.maxResponseBytes
	r.stateLock.RUnlock()

	if limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			return nil, 0, &RateLimitError{Method: method, URL: safeURL(requestURL), Err: err}
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create %s request for %s", method, safeURL(requestURL))
	}

	r.headerLock.RLock()
	for key, value := range r.headers {
		req.Header.Set(key, value)
	}
	r.headerLock.RUnlock()
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}

	res, err := client.Do(req)
	if err != nil {
		err = unwrapURLErrors(err)
		return nil, 0, fmt.Errorf("%s request to %s failed: %w", method, safeURL(requestURL), err)
	}
	return res, maxResponseBytes, nil
}

func readResponseBody(body io.Reader, maxBytes int64, contentLength int64, requestURL string) ([]byte, error) {
	if maxBytes > 0 && contentLength > maxBytes {
		return nil, &ResponseTooLargeError{
			MaxBytes:      maxBytes,
			ContentLength: contentLength,
			URL:           safeURL(requestURL),
		}
	}

	reader := body
	if maxBytes > 0 && maxBytes < int64(^uint64(0)>>1) {
		reader = io.LimitReader(body, maxBytes+1)
	}
	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(b)) > maxBytes {
		return nil, &ResponseTooLargeError{
			MaxBytes:      maxBytes,
			ContentLength: -1,
			URL:           safeURL(requestURL),
		}
	}
	return b, nil
}

func newHTTPStatusError(res *http.Response, requestURL string) error {
	return &HTTPStatusError{
		StatusCode:        res.StatusCode,
		RetryAfterSeconds: retryAfterSeconds(res.Header.Get("Retry-After"), time.Now()),
		URL:               safeURL(requestURL),
	}
}

func responseErrorReadLimit(maxBytes int64) int64 {
	if maxBytes > 0 && maxBytes < errorBodyReadLimit {
		return maxBytes
	}
	return errorBodyReadLimit
}

func retryAfterSeconds(value string, now time.Time) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds > 0 {
			return seconds
		}
		return 0
	}
	retryAt, err := http.ParseTime(value)
	if err != nil || !retryAt.After(now) {
		return 0
	}
	seconds := int(retryAt.Sub(now) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func safeURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<invalid-url>"
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "<invalid-url>"
	}
	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
}

func unwrapURLErrors(err error) error {
	for {
		var urlError *url.Error
		if !errors.As(err, &urlError) || urlError.Err == nil || urlError.Err == err {
			return err
		}
		err = urlError.Err
	}
}
