package web

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleSubscriptionUserAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserAgent *string `json:"user_agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserAgent == nil {
		writeError(w, http.StatusBadRequest, "请提供 user_agent，留空字符串恢复默认")
		return
	}
	dto, err := s.app.SetSubscriptionUserAgent(r.PathValue("id"), r.PathValue("subId"), *req.UserAgent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dto)
}
