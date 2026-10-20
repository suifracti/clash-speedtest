package web

import (
	"context"
	"errors"
	"net/http"
)

func (s *Server) handleRefreshAccountUsage(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.RefreshAccountUsage(r.Context(), r.PathValue("id"), r.PathValue("subId"), s.config.UserAgent)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusRequestTimeout
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
