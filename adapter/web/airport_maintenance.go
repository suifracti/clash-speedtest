package web

import (
	"encoding/json"
	"github.com/faceair/clash-speedtest/core/profiles"
	"net/http"
)

func (s *Server) handleAirportMaintenance(w http.ResponseWriter, r *http.Request) {
	var settings profiles.AirportMaintenance
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "设置格式无效")
		return
	}
	if err := s.app.SaveAirportMaintenance(r.PathValue("id"), settings); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
