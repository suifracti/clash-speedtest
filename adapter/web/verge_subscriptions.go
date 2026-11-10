package web

import (
	"encoding/json"
	"net/http"

	"github.com/faceair/clash-speedtest/application"
)

func (s *Server) handleListClashVergeSubscriptions(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.ListClashVergeSubscriptions()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleImportClashVergeSubscription(w http.ResponseWriter, r *http.Request) {
	var request application.ClashVergeImportRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "无效的订阅选择")
		return
	}
	result, err := s.app.ImportClashVergeSubscription(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
