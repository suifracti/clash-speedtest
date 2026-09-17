package gui

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"gopkg.in/yaml.v2"
)

// ExportClashYAML exports a list of proxy configurations as a valid Clash YAML config.
func ExportClashYAML(proxies []map[string]any) ([]byte, error) {
	config := &speedtester.RawConfig{
		Proxies: proxies,
	}
	return yaml.Marshal(config)
}

// ExportCSV exports run node results as CSV format.
func ExportCSV(results []*history.RunNodeResult) ([]byte, error) {
	buf := new(bytes.Buffer)
	// Write UTF-8 BOM so Excel opens Chinese characters correctly
	buf.WriteString("\xEF\xBB\xBF")

	w := csv.NewWriter(buf)
	header := []string{
		"序号",
		"节点名称",
		"协议类型",
		"国家地区",
		"真实延迟 (ms)",
		"抖动 (ms)",
		"丢包率 (%)",
		"下载速度 (MB/s)",
		"上传速度 (MB/s)",
		"Antigravity状态",
		"Antigravity详情",
		"出口国家",
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}

	for i, r := range results {
		latencyStr := "N/A"
		if r.LatencyMs > 0 {
			latencyStr = fmt.Sprintf("%d", r.LatencyMs)
		}
		jitterStr := "N/A"
		if r.JitterMs > 0 {
			jitterStr = fmt.Sprintf("%d", r.JitterMs)
		}
		dlSpeedStr := fmt.Sprintf("%.2f", r.DownloadSpeedMBps)
		if r.DownloadError != "" {
			dlSpeedStr = r.DownloadError
		}
		ulSpeedStr := fmt.Sprintf("%.2f", r.UploadSpeedMBps)
		if r.UploadError != "" {
			ulSpeedStr = r.UploadError
		}

		statusLabel := r.AntigravityStatus
		switch r.AntigravityStatus {
		case "available":
			statusLabel = "可用"
		case "blocked":
			statusLabel = "地区不支持"
		case "token_expired":
			statusLabel = "凭证失效"
		case "unreachable":
			statusLabel = "节点不通"
		case "unknown":
			statusLabel = "未知"
		}

		country := r.CountryCode
		if r.CountryFlag != "" {
			country = r.CountryFlag + " " + country
		}

		row := []string{
			fmt.Sprintf("%d", i+1),
			r.ProxyName,
			r.ProxyType,
			country,
			latencyStr,
			jitterStr,
			fmt.Sprintf("%.1f", r.PacketLoss),
			dlSpeedStr,
			ulSpeedStr,
			statusLabel,
			strings.ReplaceAll(r.AntigravityDetail, "\n", " "),
			r.ExitCountry,
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
