package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/faceair/clash-speedtest/application"
)

func (s *Server) handleListSubscriptionRefreshJobs(w http.ResponseWriter, _ *http.Request) {
	jobs, err := s.app.ListSubscriptionRefreshJobs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleStartSubscriptionRefreshJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var req application.SubscriptionRefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "无效的订阅刷新请求")
		return
	}
	job, err := s.app.StartSubscriptionRefreshJob(req, s.config.UserAgent)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "运行中") || strings.Contains(err.Error(), "关闭") {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleGetSubscriptionRefreshJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.app.GetSubscriptionRefresh(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCancelSubscriptionRefreshJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.app.CancelSubscriptionRefresh(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}
