package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

type HTTPDelegate interface {
	Do(req *http.Request) (*http.Response, error)
}

type HTTPClient struct {
	base    string
	m       sync.RWMutex
	headers http.Header
	client  HTTPDelegate
}

func NewHTTPClient(base string, httpDelegate ...HTTPDelegate) *HTTPClient {
	pathBase := base
	if strings.HasSuffix(base, "/") {
		pathBase = strings.TrimRight(base, "/")
	}
	var delegate HTTPDelegate
	if len(httpDelegate) != 0 {
		delegate = httpDelegate[0]
	} else {
		delegate = &http.Client{}
	}
	return &HTTPClient{
		base:    pathBase,
		headers: http.Header{},
		client:  delegate,
	}
}

func (c *HTTPClient) SetHeader(key, val string) {
	c.m.Lock()
	defer c.m.Unlock()
	c.headers.Add(key, val)
}

func (c *HTTPClient) UnsetHeader(key string) {
	c.m.Lock()
	defer c.m.Unlock()
	c.headers.Del(key)
}

type RequestContext struct {
	c       *HTTPClient
	ctx     context.Context
	method  string
	path    string
	body    io.Reader
	headers http.Header
	query   map[string]string
	errors  []error
}

type ResponseHelper struct {
	*http.Response
}

func (c *HTTPClient) startRequest(ctx context.Context, method string, path string) *RequestContext {
	headers := http.Header{}
	for k, l := range c.headers {
		for _, v := range l {
			headers.Add(k, v)
		}
	}
	return &RequestContext{
		c:       c,
		ctx:     ctx,
		method:  method,
		query:   map[string]string{},
		headers: headers,
		path:    path,
	}
}

func (c *HTTPClient) Get(ctx context.Context, path string, args ...any) *RequestContext {
	return c.startRequest(ctx, "GET", fmt.Sprintf(path, args...))
}

func (c *HTTPClient) Post(ctx context.Context, path string, args ...any) *RequestContext {
	return c.startRequest(ctx, "POST", fmt.Sprintf(path, args...))
}

func (c *HTTPClient) Put(ctx context.Context, path string, args ...any) *RequestContext {
	return c.startRequest(ctx, "PUT", fmt.Sprintf(path, args...))
}

func (c *HTTPClient) Delete(ctx context.Context, path string, args ...any) *RequestContext {
	return c.startRequest(ctx, "DELETE", fmt.Sprintf(path, args...))
}

func (rc *RequestContext) handleErr(err error) {
	if err == nil {
		return
	}
	rc.errors = append(rc.errors, err)
}

func (rc *RequestContext) WithQuery(key string, val string) *RequestContext {
	rc.query[key] = val
	return rc
}

func (rc *RequestContext) WithJSON(data any) *RequestContext {
	raw, err := json.Marshal(data)
	rc.handleErr(err)
	if err == nil {
		rc.body = bytes.NewReader(raw)
	}
	return rc
}

func (rc *RequestContext) WithHeader(key, val string) *RequestContext {
	rc.headers.Add(key, val)
	return rc
}

func (rc *RequestContext) Poll(onResponse func(*ResponseHelper) (bool, error), pollingInterval time.Duration, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(rc.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			res, err := rc.Do()
			if err != nil {
				return err
			}
			ok, err := onResponse(res)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}
}

func (rc *RequestContext) Do() (*ResponseHelper, error) {
	// handle previous errors
	if len(rc.errors) != 0 {
		msg := ""
		for _, e := range rc.errors {
			msg += fmt.Sprintf("%v;", e)
		}
		return nil, fmt.Errorf("failed to construct request: %s", msg)
	}

	// path
	urlStr, err := neturl.JoinPath(rc.c.base, strings.Split(rc.path, "/")...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to construct URL, base: %s, path: %s", rc.c.base, rc.path)
	}

	// new request
	req, err := http.NewRequest(rc.method, urlStr, rc.body)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to construct request, method: %s, url: %s", rc.method, urlStr)
	}

	// query
	query := req.URL.Query()
	for k, v := range rc.query {
		query.Add(k, v)
	}
	req.URL.RawQuery = query.Encode()

	// headers
	req.Header = rc.headers

	// send request
	res, err := rc.c.client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to send request, method: %s, path: %s, query: %v, headers: %v", rc.method, rc.path, rc.query, rc.headers)
	}
	return &ResponseHelper{res}, nil
}

func (rh *ResponseHelper) JSON(data any) error {
	raw, err := io.ReadAll(rh.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read body from HTTP response")
	}
	if err := json.Unmarshal(raw, data); err != nil {
		return errors.Wrapf(err, "failed to unmarshal JSON, body: %s", string(raw))
	}
	return nil
}

func (rh *ResponseHelper) ExpectStatusWithMessage(msg string, statusCodes ...int) error {
	for _, c := range statusCodes {
		if rh.StatusCode == c {
			return nil
		}
	}
	if len(msg) == 0 {
		return fmt.Errorf("unexpected status code: %d, expecting: %v", rh.StatusCode, statusCodes)
	}
	return fmt.Errorf("%s, unexpected status code: %d, expecting: %v", msg, rh.StatusCode, statusCodes)
}

func (rh *ResponseHelper) ExpectStatus(statusCodes ...int) error {
	return rh.ExpectStatusWithMessage("", statusCodes...)
}
