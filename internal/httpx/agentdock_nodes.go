package httpx

import (
	"errors"
	"net/http"

	"github.com/uvwt/nexusdock/internal/agentdock"
)

func (s *Server) registerAgentDockNodeRoutes(mux *http.ServeMux, protected func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /v1/runtime/nodes", protected(s.agentDockNodeList))
	mux.HandleFunc("POST /v1/runtime/nodes", protected(s.agentDockNodeCreate))
	mux.HandleFunc("GET /v1/runtime/nodes/{nodeID}", protected(s.agentDockNodeGet))
	mux.HandleFunc("PATCH /v1/runtime/nodes/{nodeID}", protected(s.agentDockNodeUpdate))
	mux.HandleFunc("DELETE /v1/runtime/nodes/{nodeID}", protected(s.agentDockNodeDelete))
	mux.HandleFunc("POST /v1/runtime/nodes/{nodeID}/probe", protected(s.agentDockNodeProbe))
}

func (s *Server) agentDockNodeList(w http.ResponseWriter, r *http.Request) {
	if s.agentDock == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENTDOCK_NODE_STORE_UNAVAILABLE", "AgentDock 节点存储不可用")
		return
	}
	nodes, err := s.agentDock.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AGENTDOCK_NODE_LIST_FAILED", "无法读取 AgentDock 节点")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "nodes": nodes, "count": len(nodes)})
}

func (s *Server) agentDockNodeCreate(w http.ResponseWriter, r *http.Request) {
	if s.agentDock == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENTDOCK_NODE_STORE_UNAVAILABLE", "AgentDock 节点存储不可用")
		return
	}
	var request agentdock.CreateInput
	if !decodeJSON(w, r, &request) {
		return
	}
	node, err := s.agentDock.Create(r.Context(), request)
	if err != nil {
		writeAgentDockNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "node": node})
}

func (s *Server) agentDockNodeGet(w http.ResponseWriter, r *http.Request) {
	if s.agentDock == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENTDOCK_NODE_STORE_UNAVAILABLE", "AgentDock 节点存储不可用")
		return
	}
	node, err := s.agentDock.Get(r.Context(), r.PathValue("nodeID"))
	if err != nil {
		writeAgentDockNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "node": node})
}

func (s *Server) agentDockNodeUpdate(w http.ResponseWriter, r *http.Request) {
	if s.agentDock == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENTDOCK_NODE_STORE_UNAVAILABLE", "AgentDock 节点存储不可用")
		return
	}
	var request agentdock.UpdateInput
	if !decodeJSON(w, r, &request) {
		return
	}
	node, err := s.agentDock.Update(r.Context(), r.PathValue("nodeID"), request)
	if err != nil {
		writeAgentDockNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "node": node})
}

func (s *Server) agentDockNodeDelete(w http.ResponseWriter, r *http.Request) {
	if s.agentDock == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENTDOCK_NODE_STORE_UNAVAILABLE", "AgentDock 节点存储不可用")
		return
	}
	id := r.PathValue("nodeID")
	if err := s.agentDock.Delete(r.Context(), id); err != nil {
		writeAgentDockNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "node_id": id, "deleted": true})
}

func (s *Server) agentDockNodeProbe(w http.ResponseWriter, r *http.Request) {
	client, err := s.agentDockRuntimeClient(r.Context(), r.PathValue("nodeID"))
	if err != nil {
		writeJSON(w, runtimeErrorHTTPStatus(err), runtimeUnavailablePayload(err))
		return
	}
	health, err := client.request(r.Context(), http.MethodGet, "/healthz", nil, nil)
	if err != nil {
		writeJSON(w, runtimeErrorHTTPStatus(err), runtimeUnavailablePayload(err))
		return
	}
	_, err = client.request(r.Context(), http.MethodGet, "/internal/runtime/tasks", runtimeQueryLimitStatus(1, ""), nil)
	if err != nil {
		writeJSON(w, runtimeErrorHTTPStatus(err), runtimeUnavailablePayload(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "available": true, "node": client.node, "health": health,
	})
}

func writeAgentDockNodeError(w http.ResponseWriter, err error) {
	var validationError agentdock.ValidationError
	switch {
	case errors.Is(err, agentdock.ErrNodeNotFound):
		writeError(w, http.StatusNotFound, "AGENTDOCK_NODE_NOT_FOUND", err.Error())
	case errors.Is(err, agentdock.ErrNodeExists):
		writeError(w, http.StatusConflict, "AGENTDOCK_NODE_EXISTS", err.Error())
	case errors.Is(err, agentdock.ErrNodeDisabled):
		writeError(w, http.StatusConflict, "AGENTDOCK_NODE_DISABLED", err.Error())
	case errors.As(err, &validationError):
		writeError(w, http.StatusBadRequest, "INVALID_AGENTDOCK_NODE", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "AGENTDOCK_NODE_OPERATION_FAILED", "无法完成 AgentDock 节点操作")
	}
}
