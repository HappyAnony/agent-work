// SPDX-License-Identifier: AGPL-3.0-or-later
package aiflow

import (
	"context"
	"github.com/sashabaranov/go-openai"
	"net/http"
	"net/url"
	"strings"
	"time"
	"wiki-kernel/conf"
)

type sdkClient struct {
	c         *openai.Client
	v         conf.OpenAI
	transport *http.Transport
}
type agentTransport struct {
	next  http.RoundTripper
	agent string
}

func (t agentTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.Header = r.Header.Clone()
	copy.Header.Add("User-Agent", t.agent)
	return t.next.RoundTrip(copy)
}
func OpenAI(v conf.OpenAI) (Client, error) {
	return openAIClient(v, nil)
}

// Device configuration authorizes the configured endpoint, including where
// prompts may be sent. A redirect is a different destination.
func OpenAIForDevice(v conf.OpenAI) (Client, error) {
	return openAIClient(v, func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse })
}

func openAIClient(v conf.OpenAI, redirect func(*http.Request, []*http.Request) error) (Client, error) {
	cfg := openai.DefaultConfig(v.APIKey)
	if v.APIProvider == "Azure" {
		cfg = openai.DefaultAzureConfig(v.APIKey, v.APIBaseURL)
		cfg.APIVersion = v.APIVersion
	}
	transport := &http.Transport{}
	if v.APIProxy != "" {
		proxy, e := url.Parse(v.APIProxy)
		if e != nil || proxy.Scheme == "" || proxy.Host == "" {
			return nil, ErrConfig
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	cfg.HTTPClient = &http.Client{Transport: agentTransport{transport, v.APIUserAgent}, CheckRedirect: redirect}
	cfg.BaseURL = v.APIBaseURL
	return &sdkClient{c: openai.NewClientWithConfig(cfg), v: v, transport: transport}, nil
}
func (c *sdkClient) Close() { c.transport.CloseIdleConnections() }
func (c *sdkClient) Chat(ctx context.Context, msg string, history []string) (Part, error) {
	var messages []openai.ChatCompletionMessage
	for _, v := range history {
		if v != "" {
			messages = append(messages, openai.ChatCompletionMessage{Role: "user", Content: v})
		}
	}
	if msg != "" {
		messages = append(messages, openai.ChatCompletionMessage{Role: "user", Content: msg})
	}
	if len(messages) == 0 {
		return Part{Stop: true}, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.v.APITimeout)*time.Second)
	defer cancel()
	response, e := c.c.CreateChatCompletion(ctx, openai.ChatCompletionRequest{Model: c.v.APIModel, MaxCompletionTokens: c.v.APIMaxTokens, Temperature: float32(c.v.APITemperature), Messages: messages})
	if e != nil {
		if ctx.Err() != nil {
			return Part{}, ctx.Err()
		}
		return Part{}, ErrProvider
	}
	if len(response.Choices) == 0 {
		return Part{Stop: true}, nil
	}
	v := response.Choices[0]
	return Part{Text: strings.TrimSpace(v.Message.Content), Stop: v.FinishReason != "length"}, nil
}
