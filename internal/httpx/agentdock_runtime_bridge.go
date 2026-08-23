package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/uvwt/nexusdock/internal/agentdock"
)

type agentDockRuntimeError struct {
	Status       int    `json:"-"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	UpstreamCode string `json:"upstream_code,omitempty"`
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
	if s.agentDockHub == nil {
		return nil, agentDockRuntimeError{Code: "AGENTDOCK_CONNECTION_UNAVAILABLE", Message: "AgentDock 节点连接服务不可用"}
	}
	if s.agentDock != nil {
		if _, err := s.agentDock.Get(ctx, nodeID); err != nil {
			if errors.Is(err, agentdock.ErrNodeNotFound) {
				return nil, agentDockRuntimeError{Status: http.StatusNotFound, Code: "AGENTDOCK_NODE_NOT_FOUND", Message: err.Error()}
			}
			return nil, agentDockRuntimeError{Code: "AGENTDOCK_NODE_LOOKUP_FAILED", Message: err.Error()}
		}
	}
	arguments := map[string]any{"method": method, "path": path}
	if len(query) > 0 {
		arguments["query"] = query
	}
	if len(requestBody) > 0 {
		arguments["body"] = json.RawMessage(requestBody)
	}
	result, err := s.agentDockHub.Invoke(ctx, nodeID, "runtime.request", arguments)
	if err != nil {
		return nil, agentDockRuntimeError{Code: "AGENTDOCK_RUNTIME_UNREACHABLE", Message: err.Error()}
	}
	return result, nil
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
	detail := map[string]any{"code": code, "message": message}
	if rtErr.UpstreamCode != "" {
		detail["upstream_code"] = rtErr.UpstreamCode
	}
	return map[string]any{"ok": false, "available": false, "source": "agentdock-runtime-api", "error": detail}
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

func isRuntimeUnavailable(err error) bool {
	var runtimeErr agentDockRuntimeError
	return errors.As(err, &runtimeErr)
}
