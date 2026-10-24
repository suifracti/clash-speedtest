package web

import "net/http"

func (s *Server) handleSubscriptionUsage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	report, err := s.app.GetSubscriptionUsage(r.Context(), q.Get("from"), q.Get("until"), q.Get("period"), q.Get("account"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleSubscriptionUsageState(w http.ResponseWriter, r *http.Request) {
	state, err := s.app.GetSubscriptionRefreshState(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleRefreshSubscriptionUsage(w http.ResponseWriter, r *http.Request) {
	state, err := s.app.UpdateSubscriptionUsage(r.Context(), s.config.UserAgent)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}
