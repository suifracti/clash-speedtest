package web

import (
	"encoding/json"
	"github.com/faceair/clash-speedtest/application"
	"net/http"
)

func (s *Server) handleClashNodeStatus(w http.ResponseWriter, r *http.Request) {
	var scope application.WorkbenchLatencyBatchSelection
	if err := json.NewDecoder(r.Body).Decode(&scope); err != nil {
		writeError(w, 400, "无效的节点")
		return
	}
	result, err := s.app.GetClashNodeStatus(r.Context(), scope)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) handleClashCompare(w http.ResponseWriter, r *http.Request) {
	var scope application.WorkbenchLatencyBatchSelection
	if err := json.NewDecoder(r.Body).Decode(&scope); err != nil {
		writeError(w, 400, "无效的节点")
		return
	}
	result, err := s.app.CompareClashNode(r.Context(), scope)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) handleClashUsage(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.app.GetClashUsage(r.Context()))
}
func (s *Server) handleClashGroups(w http.ResponseWriter, r *http.Request) {
	var scope application.WorkbenchLatencyBatchSelection
	if err := json.NewDecoder(r.Body).Decode(&scope); err != nil {
		writeError(w, 400, "无效的节点")
		return
	}
	result, err := s.app.GetClashNodeGroups(r.Context(), scope)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) handleClashSelect(w http.ResponseWriter, r *http.Request) {
	var req application.ClashSelectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "无效的选择")
		return
	}
	result, err := s.app.UseClashNode(r.Context(), req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
