// Package httpx 封装出站 HTTP 调用：超时、重试、结构化结果与耗时统计。
// 联调任务的「请求步骤」直接复用它，保证与服务内部调用行为一致。
package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"time"
)

// Client 带默认头、重试与耗时统计的 HTTP 客户端。
type Client struct {
	hc           *http.Client
	defaultHeads map[string]string
	maxBodyBytes int64
}

// Options 构造参数。
type Options struct {
	Timeout            time.Duration
	MaxIdleConns       int
	InsecureSkipVerify bool
	DefaultHeaders     map[string]string
	MaxBodyBytes       int64
}

// New 创建客户端。
func New(opt Options) *Client {
	if opt.Timeout <= 0 {
		opt.Timeout = 15 * time.Second
	}
	if opt.MaxIdleConns <= 0 {
		opt.MaxIdleConns = 200
	}
	if opt.MaxBodyBytes <= 0 {
		opt.MaxBodyBytes = 4 << 20 // 4MB
	}
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          opt.MaxIdleConns,
		MaxIdleConnsPerHost:   opt.MaxIdleConns / 2,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	if opt.InsecureSkipVerify {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // 联调环境允许自签证书
	}
	return &Client{
		hc:           &http.Client{Transport: tr, Timeout: opt.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		defaultHeads: opt.DefaultHeaders,
		maxBodyBytes: opt.MaxBodyBytes,
	}
}

// Do 执行请求并读取完整响应。req 的 Body 若需重试，调用方需自行保证可重放。
func (c *Client) Do(ctx context.Context, req *http.Request, body []byte) (*http.Response, []byte, time.Duration, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	if body != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	}
	for k, v := range c.defaultHeads {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
	start := time.Now()
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, nil, time.Since(start), err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBodyBytes))
	return resp, data, time.Since(start), err
}
