package web

import "net/http"

func (s *Server) handleRefreshAccountUsage(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.RefreshAccountUsage(r.Context(), r.PathValue("id"), r.PathValue("subId"), s.config.UserAgent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
