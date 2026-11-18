package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/faceair/clash-speedtest/core/history"
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
func (s *Server) handleListManualMeasurementRounds(w http.ResponseWriter, r *http.Request) {
	page, err := s.app.ListManualMeasurementRounds(r.Context(), r.URL.Query().Get("before"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}
func (s *Server) handleReadManualMeasurementRound(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeError(w, http.StatusBadRequest, "invalid item offset")
			return
		}
	}
	detail, err := s.app.ReadManualMeasurementRound(r.Context(), r.PathValue("id"), offset)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "measurement round not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
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
