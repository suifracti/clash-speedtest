package application

import (
	"context"
	"github.com/faceair/clash-speedtest/core/history"
)

func (s *AppService) PreviousWorkbenchDownloadAttempt(ctx context.Context, id string, query WorkbenchDownloadHistoryQuery) (*history.WorkbenchDownloadAttempt, error) {
	current, err := s.GetWorkbenchDownloadAttempt(ctx, id, query)
	if err != nil {
		return nil, err
	}
	return s.historyStore.DB().PreviousWorkbenchDownloadAttempt(ctx, current)
}

func (s *AppService) PreviousWorkbenchPublicServiceAttempt(ctx context.Context, id string, query WorkbenchPublicServiceHistoryQuery) (*history.PublicServiceAttempt, error) {
	current, err := s.GetWorkbenchPublicServiceAttempt(ctx, id, query)
	if err != nil {
		return nil, err
	}
	return s.historyStore.DB().PreviousPublicServiceAttempt(ctx, current)
}
