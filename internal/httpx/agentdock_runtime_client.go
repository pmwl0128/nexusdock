package httpx

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

	"github.com/uvwt/nexusdock/internal/agentdock"
)

const maxAgentDockRuntimeResponseBytes = 8 << 20

type agentDockRuntimeClient struct {
	endpoint string
	token    string
	client   *http.Client
	node     agentdock.Node
}

type agentDockRuntimeError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e agentDockRuntimeError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return "AgentDock Runtime API unavailable"
}

func (s *Server) agentDockRuntimeClient(ctx context.Context, nodeID string) (*agentDockRuntimeClient, error) {
	if s.agentDock == nil {
		return nil, agentDockRuntimeError{Code: "AGENTDOCK_NODE_STORE_UNAVAILABLE", Message: "AgentDock 节点存储不可用"}
	}
	credentials, err := s.agentDock.Credentials(ctx, nodeID)
	if err != nil {
		switch {
		case errors.Is(err, agentdock.ErrNodeNotFound):
			return nil, agentDockRuntimeError{Status: http.StatusNotFound, Code: "AGENTDOCK_NODE_NOT_FOUND", Message: err.Error()}
		case errors.Is(err, agentdock.ErrNodeDisabled):
			return nil, agentDockRuntimeError{Status: http.StatusConflict, Code: "AGENTDOCK_NODE_DISABLED", Message: err.Error()}
		default:
			return nil, agentDockRuntimeError{Code: "AGENTDOCK_NODE_CREDENTIALS_UNAVAILABLE", Message: "AgentDock 节点凭据不可用"}
		}
	}
	timeout := time.Duration(credentials.Node.TimeoutSeconds) * time.Second
	return &agentDockRuntimeClient{
		endpoint: credentials.Node.Endpoint,
		token:    credentials.Token,
		client: &http.Client{
			Timeout: timeout,
			// 节点地址必须直接指向目标 AgentDock，禁止重定向掩盖配置错误或扩大凭据转发范围。
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
		node: credentials.Node,
	}, nil
}

func (s *Server) runtimeGet(ctx context.Context, nodeID, path string, query url.Values) (map[string]any, error) {
	return s.runtimeRequest(ctx, nodeID, http.MethodGet, path, query, nil)
}

func (s *Server) runtimeDelete(ctx context.Context, nodeID, path string) (map[string]any, error) {
	return s.runtimeRequest(ctx, nodeID, http.MethodDelete, path, nil, nil)
}

func (s *Server) runtimePost(ctx context.Context, nodeID, path string, payload any) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode AgentDock Runtime request: %w", err)
	}
	return s.runtimeRequest(ctx, nodeID, http.MethodPost, path, nil, body)
}

func (s *Server) runtimeRequest(ctx context.Context, nodeID, method, path string, query url.Values, requestBody []byte) (map[string]any, error) {
	client, err := s.agentDockRuntimeClient(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	return client.request(ctx, method, path, query, requestBody)
}

func (c *agentDockRuntimeClient) request(ctx context.Context, method, path string, query url.Values, requestBody []byte) (map[string]any, error) {
	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if len(requestBody) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, agentDockRuntimeError{Code: "AGENTDOCK_RUNTIME_UNREACHABLE", Message: err.Error()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAgentDockRuntimeResponseBytes+1))
	if err != nil {
		return nil, agentDockRuntimeError{Status: resp.StatusCode, Code: "AGENTDOCK_RUNTIME_BAD_RESPONSE", Message: err.Error()}
	}
	if len(data) > maxAgentDockRuntimeResponseBytes {
		return nil, agentDockRuntimeError{Status: resp.StatusCode, Code: "AGENTDOCK_RUNTIME_BAD_RESPONSE", Message: "AgentDock Runtime 响应超过 8 MiB 限制"}
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil || body == nil {
		if err == nil {
			err = errors.New("AgentDock Runtime 返回了空 JSON 对象")
		}
		return nil, agentDockRuntimeError{Status: resp.StatusCode, Code: "AGENTDOCK_RUNTIME_BAD_RESPONSE", Message: err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, agentDockRuntimeError{Status: resp.StatusCode, Code: firstNonEmptyString(opsString(body["code"]), fmt.Sprintf("HTTP_%d", resp.StatusCode)), Message: firstNonEmptyString(opsString(body["error"]), resp.Status)}
	}
	return body, nil
}

func runtimeQueryLimitStatus(limit int, status string) url.Values {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if strings.TrimSpace(status) != "" && status != "all" {
		query.Set("status", status)
	}
	return query
}

func runtimeUnavailablePayload(err error) map[string]any {
	code := "AGENTDOCK_RUNTIME_UNAVAILABLE"
	message := "AgentDock Runtime API 不可用"
	var rtErr agentDockRuntimeError
	if err != nil {
		if converted, ok := err.(agentDockRuntimeError); ok {
			rtErr = converted
		} else if converted, ok := err.(*agentDockRuntimeError); ok {
			rtErr = *converted
		}
		if rtErr.Code != "" {
			code = rtErr.Code
		}
		if err.Error() != "" {
			message = err.Error()
		}
	}
	return map[string]any{"ok": false, "available": false, "source": "agentdock-runtime-api", "code": code, "error": message}
}

func runtimeErrorHTTPStatus(err error) int {
	status := http.StatusServiceUnavailable
	switch converted := err.(type) {
	case agentDockRuntimeError:
		status = converted.Status
	case *agentDockRuntimeError:
		status = converted.Status
	}
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
		return status
	default:
		return http.StatusServiceUnavailable
	}
}
