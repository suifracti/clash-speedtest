package application

import (
	"context"
	"fmt"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net/url"
	"time"
)

func sameDownloadSource(a, b string) bool {
	x, err := url.Parse(a)
	if err != nil {
		return false
	}
	y, err := url.Parse(b)
	if err != nil {
		return false
	}
	return x.Scheme == y.Scheme && x.Host == y.Host && x.Path == y.Path
}

// Admission only: no sleep, no new retry task and no additional source request.
// The existing round skips this item with an explicit reason and serves others.
func (s *AppService) checkDownloadRetryAfter(ctx context.Context, req WorkbenchDownloadTestRequest, target string) error {
	page, err := s.historyStore.QueryWorkbenchDownloadAttempts(ctx, history.WorkbenchDownloadFilter{ProfileID: req.ProfileID, NodeKey: req.NodeKey, NodeIdentityKey: req.NodeIdentityKey, ConfigRevisionKey: req.ConfigRevisionKey, Limit: 16})
	if err != nil {
		return fmt.Errorf("无法核对测速源等待条件，本次未执行: %w", err)
	}
	for _, a := range page.Attempts {
		if a.Result == nil || a.Rule.Method != "GET" || !sameDownloadSource(a.Rule.TargetURL, target) {
			continue
		}
		r := a.Result
		// Newer completed response supersedes this source's previous cooldown.
		if r.HTTPStatus == nil {
			continue
		}
		if *r.HTTPStatus != 429 {
			return nil
		}
		delay := r.RetryAfterSeconds
		if delay < 60 {
			delay = 60
		} // Minimum admission pause, not an automatic retry.
		elapsed := int64(time.Since(r.FinishedAt).Seconds())
		if elapsed < 0 {
			elapsed = 0
		}
		remaining := delay - elapsed
		if remaining > 0 {
			if delay > 3600 {
				return monitor.NewValidationError(fmt.Sprintf("测速源要求等待，超过 3600 秒冷却预算；本次未执行、不自动排队或重试；服务器期限还剩约 %d 秒，不提前重试", remaining))
			}
			return monitor.NewValidationError(fmt.Sprintf("测速源要求等待，剩余约 %d 秒；本次未执行，不判为节点故障，等待期内不再请求同一测速源", remaining))
		}
		return nil
	}
	return nil
}
