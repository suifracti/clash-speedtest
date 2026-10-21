package publicservice

import (
	"context"
	"net/http"
	"time"
)

func googleRegionRule() Rule {
	r, _ := RuleFor("youtube_premium")
	r.ServiceID, r.Name, r.Category, r.ResultKind = "google_region", "Google 判定地区", "出口与 IP", "exit_profile"
	r.RuleVersion = 2
	r.SuccessCriterion = "读取 YouTube 返回的 Google 地区字段；不判断 Premium 或账号可用性"
	return r
}
func checkGoogleRegion(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	r := checkYouTubePremium(ctx, client, rule, started)
	if region := r.Details["service_region"]; region != "" && r.Details["final_url"] != "" {
		r.Outcome = "profiled"
		r.Summary = "Google 判定地区：" + region
		r.ErrorMessage = ""
		r.FailurePhase = ""
	}
	return r
}
