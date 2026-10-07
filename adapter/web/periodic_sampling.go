package web

import (
	"encoding/json"
	"github.com/faceair/clash-speedtest/application"
	"net/http"
)

func (s *Server) handleGetPeriodicSampling(w http.ResponseWriter, r *http.Request) {
	status, err := s.app.GetPeriodicSampling()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}
func (s *Server) handleConfigurePeriodicSampling(w http.ResponseWriter, r *http.Request) {
	var cfg application.PeriodicSamplingConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, err := s.app.ConfigurePeriodicSampling(cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handlePeriodicSample(w http.ResponseWriter, r *http.Request) {
	var req application.PeriodicSampleRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	result, err := s.app.RunPeriodicSamplingSample(r.Context(), req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) handlePeriodicChecks(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.GetPeriodicServiceChecks(r.Context(), r.URL.Query().Get("profile_id"), r.URL.Query().Get("node_key"))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) handlePausePeriodicMeasurements(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.PausePeriodicMeasurementsForSample()
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
