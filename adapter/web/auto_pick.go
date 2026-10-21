package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/faceair/clash-speedtest/core/autopick"
)

func (s *Server) handleGetAutoPick(w http.ResponseWriter, r *http.Request) {
	status, err := s.app.GetAutoPickStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleSaveAutoPick(w http.ResponseWriter, r *http.Request) {
	var cfg autopick.Config
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, err := s.app.SaveAutoPickConfig(cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRunAutoPick(w http.ResponseWriter, r *http.Request) {
	status, err := s.app.RunAutoPickNow()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}

func (s *Server) handleListAutoPickReports(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	reports, err := s.app.ListAutoPickReports(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, reports)
}

func (s *Server) handleGetAutoPickReport(w http.ResponseWriter, r *http.Request) {
	report, err := s.app.GetAutoPickReport(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
