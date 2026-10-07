package web

import (
	"encoding/json"
	"github.com/faceair/clash-speedtest/core/history"
	"net/http"
	"strconv"
)

func (s *Server) handleMeasurementRounds(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	result, err := s.app.ListMeasurementRoundNodes(r.Context(), q.Get("profile_id"), q.Get("node_key"), q.Get("node_identity_key"), q.Get("config_revision_key"), limit)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) handleCreateMeasurementRound(w http.ResponseWriter, r *http.Request) {
	var plan history.MeasurementRound
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&plan); err != nil {
		writeError(w, 400, "invalid bounded round plan")
		return
	}
	if err := s.app.CreateManualMeasurementRound(r.Context(), plan); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"round_id": plan.RoundID})
}
func (s *Server) handleFinishMeasurementRound(w http.ResponseWriter, r *http.Request) {
	var body struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		writeError(w, 400, "invalid round state")
		return
	}
	if err := s.app.FinishManualMeasurementRound(r.Context(), r.PathValue("id"), body.State); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
