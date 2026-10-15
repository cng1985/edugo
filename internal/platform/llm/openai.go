package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/shared/apperr"
)

type openAI struct {
	cfg    *config.Config
	client *http.Client
}

func newOpenAI(cfg *config.Config) *openAI {
	return &openAI{cfg: cfg, client: &http.Client{Timeout: cfg.AI.RequestTimeout}}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatBody struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	Stream         bool          `json:"stream,omitempty"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}

func (o *openAI) complete(ctx context.Context, profile config.AIProfile, req ChatRequest) (ChatResult, int, error) {
	body := chatBody{
		Model: profile.Model, Temperature: 0.2, MaxTokens: profile.MaxOutputTokens,
		Messages: []chatMessage{{Role: "system", Content: req.System}, {Role: "user", Content: req.User}},
	}
	if req.JSONMode {
		body.ResponseFormat = &struct {
			Type string `json:"type"`
		}{Type: "json_object"}
	}
	var last error
	retries := 0
	for attempt := 0; attempt <= o.cfg.AI.MaxRetries; attempt++ {
		if attempt > 0 {
			retries = attempt
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		res, retry, err := o.doComplete(ctx, body)
		if err == nil {
			return res, retries, nil
		}
		last = err
		if !retry || attempt == o.cfg.AI.MaxRetries {
			break
		}
	}
	return ChatResult{}, retries, last
}

func (o *openAI) doComplete(ctx context.Context, body chatBody) (ChatResult, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, o.cfg.AI.RequestTimeout)
	defer cancel()
	raw, status, upstream, err := o.postBytes(ctx, "/chat/completions", body)
	if err != nil {
		return ChatResult{}, true, err
	}
	if status == 429 || status >= 500 {
		return ChatResult{}, true, apperr.Unavailable("MODEL_UPSTREAM", "模型服务暂时不可用")
	}
	if status == 401 || status == 403 {
		return ChatResult{}, false, apperr.Unavailable("MODEL_AUTH", "模型服务拒绝认证")
	}
	if status >= 400 {
		return ChatResult{}, false, apperr.Unavailable("MODEL_BAD_REQUEST", "模型请求被拒绝")
	}
	var parsed struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ChatResult{}, false, apperr.Unavailable("MODEL_BAD_RESPONSE", "模型响应无法解析")
	}
	content := ""
	if len(parsed.Choices) > 0 {
		content = parsed.Choices[0].Message.Content
	}
	res := ChatResult{Content: content, UpstreamID: firstNonEmpty(parsed.ID, upstream)}
	if parsed.Usage != nil {
		res.Usage = Usage{
			Known: true, PromptTokens: intPtr(parsed.Usage.PromptTokens),
			CompletionTokens: intPtr(parsed.Usage.CompletionTokens), TotalTokens: intPtr(parsed.Usage.TotalTokens),
		}
	}
	return res, false, nil
}

func (o *openAI) stream(ctx context.Context, profile config.AIProfile, req ChatRequest) (ChatStream, error) {
	body := chatBody{
		Model: profile.Model, Temperature: 0.2, MaxTokens: profile.MaxOutputTokens, Stream: true,
		Messages: []chatMessage{{Role: "system", Content: req.System}, {Role: "user", Content: req.User}},
	}
	ctx, cancel := context.WithTimeout(ctx, o.cfg.AI.StreamMaxDuration)
	raw, status, _, err := o.postStream(ctx, "/chat/completions", body)
	if err != nil {
		cancel()
		return nil, err
	}
	if status >= 400 {
		cancel()
		b, _ := io.ReadAll(io.LimitReader(raw, 512))
		raw.Close()
		_ = b
		if status == 429 || status >= 500 {
			return nil, apperr.Unavailable("MODEL_UPSTREAM", "模型服务暂时不可用")
		}
		return nil, apperr.Unavailable("MODEL_BAD_REQUEST", "模型请求被拒绝")
	}
	return &sseStream{rc: raw, sc: bufio.NewScanner(raw), cancel: cancel, idle: o.cfg.AI.StreamIdleTimeout}, nil
}

func (o *openAI) embed(ctx context.Context, profile config.AIProfile, text string) ([]float64, string, error) {
	body := map[string]any{"model": profile.Model, "input": text}
	raw, status, upstream, err := o.postBytes(ctx, "/embeddings", body)
	if err != nil {
		return nil, "", err
	}
	if status >= 400 {
		return nil, upstream, apperr.Unavailable("EMBEDDING_FAILED", "向量化失败")
	}
	var parsed struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Data) == 0 {
		return nil, upstream, apperr.Unavailable("EMBEDDING_FAILED", "向量响应无法解析")
	}
	return parsed.Data[0].Embedding, upstream, nil
}

func (o *openAI) postBytes(ctx context.Context, path string, body any) ([]byte, int, string, error) {
	resp, err := o.do(ctx, path, body, false)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, resp.StatusCode, resp.Header.Get("x-request-id"), apperr.Unavailable("MODEL_NETWORK", "读取模型响应失败")
	}
	return b, resp.StatusCode, resp.Header.Get("x-request-id"), nil
}

func (o *openAI) postStream(ctx context.Context, path string, body any) (io.ReadCloser, int, string, error) {
	resp, err := o.do(ctx, path, body, true)
	if err != nil {
		return nil, 0, "", err
	}
	return resp.Body, resp.StatusCode, resp.Header.Get("x-request-id"), nil
}

func (o *openAI) do(ctx context.Context, path string, body any, stream bool) (*http.Response, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(o.cfg.AI.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.cfg.AI.APIKey)
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, apperr.Unavailable("MODEL_NETWORK", "模型服务连接失败")
	}
	return resp, nil
}

type sseStream struct {
	rc     io.ReadCloser
	sc     *bufio.Scanner
	cancel context.CancelFunc
	idle   time.Duration
	usage  Usage
}

func (s *sseStream) Recv(ctx context.Context) (string, bool, Usage, error) {
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		if s.sc.Scan() {
			ch <- result{line: s.sc.Text()}
			return
		}
		ch <- result{err: s.sc.Err()}
	}()
	timer := time.NewTimer(s.idle)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", false, s.usage, ctx.Err()
	case <-timer.C:
		return "", false, s.usage, apperr.Unavailable("MODEL_TIMEOUT", "模型流空闲超时")
	case res := <-ch:
		if res.err != nil {
			return "", false, s.usage, apperr.Unavailable("MODEL_STREAM", "模型流中断")
		}
		line := strings.TrimSpace(res.line)
		if line == "" || strings.HasPrefix(line, ":") {
			return "", false, s.usage, nil
		}
		if !strings.HasPrefix(line, "data:") {
			return "", false, s.usage, nil
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return "", true, s.usage, nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return "", false, s.usage, apperr.Unavailable("MODEL_BAD_RESPONSE", "模型流片段无法解析")
		}
		if chunk.Usage != nil {
			s.usage = Usage{Known: true, PromptTokens: intPtr(chunk.Usage.PromptTokens), CompletionTokens: intPtr(chunk.Usage.CompletionTokens), TotalTokens: intPtr(chunk.Usage.TotalTokens)}
		}
		delta := ""
		done := false
		if len(chunk.Choices) > 0 {
			delta = chunk.Choices[0].Delta.Content
			if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason != "" {
				done = *chunk.Choices[0].FinishReason == "stop"
			}
		}
		return delta, done, s.usage, nil
	}
}

func (s *sseStream) Close() error {
	s.cancel()
	return s.rc.Close()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

