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
