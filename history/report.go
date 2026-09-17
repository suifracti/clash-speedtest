package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SaveHTMLReport writes the standalone interactive HTML report to specified target paths.
func SaveHTMLReport(runs []*TestRun, filePaths ...string) error {
	htmlContent, err := GenerateHTMLReport(runs)
	if err != nil {
		return err
	}
	for _, fp := range filePaths {
		if strings.TrimSpace(fp) == "" {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(fp), 0o755)
		if err := os.WriteFile(fp, []byte(htmlContent), 0o644); err != nil {
			return fmt.Errorf("write html report to %s failed: %w", fp, err)
		}
	}
	return nil
}

// GenerateHTMLReport generates a standalone, zero-dependency, self-contained HTML report.
func GenerateHTMLReport(runs []*TestRun) (string, error) {
	runsJSON, err := json.Marshal(runs)
	if err != nil {
		return "", fmt.Errorf("marshal test runs failed: %w", err)
	}

	// Safe injection into script tag
	safeJSON := strings.ReplaceAll(string(runsJSON), "</script>", "<\\/script>")

	tmpl := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Clash SpeedTest 离线可视化测速报告</title>
  <style>
    :root {
      --bg: #f8fafc;
      --card: #ffffff;
      --card-hover: #f1f5f9;
      --card-subtle: #f8fafc;
      --border: #e2e8f0;
      --border-light: #cbd5e1;
      --text: #0f172a;
      --text-muted: #64748b;
      --primary: #2563eb;
      --primary-hover: #1d4ed8;
      --primary-bg: #eff6ff;
      --success: #059669;
      --success-bg: #ecfdf5;
      --warning: #d97706;
      --warning-bg: #fffbeb;
      --danger: #dc2626;
      --danger-bg: #fef2f2;
      --purple: #7c3aed;
      --purple-bg: #f5f3ff;
      --shadow: 0 1px 3px rgba(0, 0, 0, 0.05), 0 1px 2px rgba(0, 0, 0, 0.02);
      --shadow-lg: 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -4px rgba(0, 0, 0, 0.1);
    }

    body.dark-theme {
      --bg: #18181b;
      --card: #27272a;
      --card-hover: #323236;
      --card-subtle: #202023;
      --border: #3f3f46;
      --border-light: #52525b;
      --text: #fafafa;
      --text-muted: #a1a1aa;
      --primary: #3b82f6;
      --primary-hover: #60a5fa;
      --primary-bg: rgba(59, 130, 246, 0.15);
      --success: #10b981;
      --success-bg: rgba(16, 185, 129, 0.15);
      --warning: #f59e0b;
      --warning-bg: rgba(245, 158, 11, 0.15);
      --danger: #ef4444;
      --danger-bg: rgba(239, 68, 68, 0.15);
      --purple: #8b5cf6;
      --purple-bg: rgba(139, 92, 246, 0.15);
      --shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
      --shadow-lg: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
    }

    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background-color: var(--bg);
      color: var(--text);
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
      line-height: 1.5;
      min-height: 100vh;
      padding-bottom: 4rem;
      -webkit-font-smoothing: antialiased;
      transition: background-color 0.2s ease, color 0.2s ease;
    }

    header {
      background: var(--card);
      border-bottom: 1px solid var(--border);
      position: sticky;
      top: 0;
      z-index: 40;
      padding: 0.85rem 1.5rem;
      box-shadow: var(--shadow);
      transition: background-color 0.2s ease, border-color 0.2s ease;
    }
    .header-inner {
      max-width: 1440px;
      margin: 0 auto;
      display: flex;
      align-items: center;
      justify-content: space-between;
      flex-wrap: wrap;
      gap: 1rem;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 0.75rem;
    }
    .brand-icon {
      width: 34px;
      height: 34px;
      background: var(--primary);
      border-radius: 8px;
      display: flex;
      align-items: center;
      justify-content: center;
      color: #fff;
      font-size: 16px;
      font-weight: 700;
      box-shadow: 0 2px 8px rgba(37, 99, 235, 0.3);
    }
    .brand h1 { font-size: 1.1rem; font-weight: 700; color: var(--text); }
    .brand-tag {
      font-size: 0.65rem;
      color: var(--primary);
      background: var(--primary-bg);
      padding: 0.15rem 0.45rem;
      border-radius: 4px;
      font-weight: 700;
      font-family: monospace;
    }
    .header-actions {
      display: flex;
      align-items: center;
      gap: 0.6rem;
      flex-wrap: wrap;
    }
    .run-select {
      background: var(--card-subtle);
      color: var(--text);
      border: 1px solid var(--border);
      padding: 0.45rem 0.85rem;
      border-radius: 8px;
      font-size: 0.825rem;
      outline: none;
      cursor: pointer;
      font-weight: 500;
    }
    .btn {
      background: var(--card);
      color: var(--text);
      border: 1px solid var(--border);
      padding: 0.45rem 0.85rem;
      border-radius: 8px;
      font-size: 0.825rem;
      font-weight: 600;
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: 0.4rem;
      transition: all 0.15s ease;
      text-decoration: none;
      box-shadow: 0 1px 2px rgba(0,0,0,0.04);
    }
    .btn:hover { background: var(--card-hover); border-color: var(--border-light); }
    .btn-primary { background: var(--primary); border-color: var(--primary); color: #fff; }
    .btn-primary:hover { background: var(--primary-hover); }

    .container {
      max-width: 1440px;
      margin: 1.5rem auto;
      padding: 0 1.5rem;
    }

    /* Country Tag Pill Colors */
    .c-tag {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-weight: 700;
      font-size: 10px;
      padding: 2px 6px;
      border-radius: 4px;
      letter-spacing: 0.05em;
    }
    .c-tag-hk { background: #eff6ff; color: #1d4ed8; border: 1px solid #bfdbfe; }
    .c-tag-us { background: #fdf2f8; color: #be185d; border: 1px solid #fbcfe8; }
    .c-tag-jp { background: #fff1f2; color: #be123c; border: 1px solid #fecdd3; }
    .c-tag-sg { background: #ecfdf5; color: #047857; border: 1px solid #a7f3d0; }
    .c-tag-tw { background: #f0fdf4; color: #15803d; border: 1px solid #bbf7d0; }
    .c-tag-kr { background: #f5f3ff; color: #6d28d9; border: 1px solid #ddd6fe; }
    .c-tag-uk { background: #f8fafc; color: #334155; border: 1px solid #cbd5e1; }
    .c-tag-de { background: #fefce8; color: #a16207; border: 1px solid #fef08a; }
    .c-tag-other { background: #f1f5f9; color: #475569; border: 1px solid #cbd5e1; }

    body.dark-theme .c-tag-hk { background: rgba(29, 78, 216, 0.2); color: #93c5fd; border-color: rgba(59, 130, 246, 0.3); }
    body.dark-theme .c-tag-us { background: rgba(190, 24, 93, 0.2); color: #f472b6; border-color: rgba(236, 72, 153, 0.3); }
    body.dark-theme .c-tag-jp { background: rgba(190, 18, 60, 0.2); color: #fb7185; border-color: rgba(244, 63, 94, 0.3); }
    body.dark-theme .c-tag-sg { background: rgba(4, 120, 87, 0.2); color: #34d399; border-color: rgba(16, 185, 129, 0.3); }
    body.dark-theme .c-tag-tw { background: rgba(21, 128, 61, 0.2); color: #4ade80; border-color: rgba(34, 197, 94, 0.3); }
    body.dark-theme .c-tag-kr { background: rgba(109, 40, 217, 0.2); color: #c084fc; border-color: rgba(168, 85, 247, 0.3); }
    body.dark-theme .c-tag-uk { background: rgba(71, 85, 105, 0.2); color: #cbd5e1; border-color: rgba(148, 163, 184, 0.3); }
    body.dark-theme .c-tag-de { background: rgba(161, 98, 7, 0.2); color: #fde047; border-color: rgba(234, 179, 8, 0.3); }
    body.dark-theme .c-tag-other { background: rgba(63, 63, 70, 0.3); color: #a1a1aa; border-color: rgba(113, 113, 122, 0.3); }

    /* Stats Grid */
    .stats-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      gap: 1rem;
      margin-bottom: 1.25rem;
    }
    .stat-card {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 1.25rem;
      display: flex;
      flex-direction: column;
      gap: 0.35rem;
      position: relative;
      overflow: hidden;
      box-shadow: var(--shadow);
      transition: background-color 0.2s ease, border-color 0.2s ease;
    }
    .stat-card::before {
      content: "";
      position: absolute;
      top: 0;
      left: 0;
      right: 0;
      height: 3px;
      background: var(--border);
    }
    .stat-card.c-blue::before { background: var(--primary); }
    .stat-card.c-green::before { background: var(--success); }
    .stat-card.c-purple::before { background: var(--purple); }
    .stat-card.c-amber::before { background: var(--warning); }
    .stat-title { font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-muted); }
    .stat-val { font-size: 1.65rem; font-weight: 700; color: var(--text); }
    .stat-sub { font-size: 0.75rem; color: var(--text-muted); }

    /* Filter Bar */
    .filter-card {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 1rem 1.25rem;
      margin-bottom: 1.25rem;
      display: flex;
      flex-direction: column;
      gap: 0.75rem;
      box-shadow: var(--shadow);
      transition: background-color 0.2s ease, border-color 0.2s ease;
    }
    .filter-row {
      display: flex;
      align-items: center;
      gap: 0.75rem;
      flex-wrap: wrap;
    }
    .filter-label { font-size: 0.78rem; font-weight: 600; color: var(--text-muted); min-width: 68px; }
    .search-input {
      background: var(--card-subtle);
      border: 1px solid var(--border);
      color: var(--text);
      padding: 0.45rem 0.85rem;
      border-radius: 8px;
      font-size: 0.825rem;
      flex: 1;
      min-width: 220px;
      outline: none;
      transition: border-color 0.15s ease;
    }
    .search-input:focus { border-color: var(--primary); }
    .pill-group {
      display: flex;
      flex-wrap: wrap;
      gap: 0.4rem;
      align-items: center;
    }
    .pill {
      background: var(--card-subtle);
      color: var(--text-muted);
      border: 1px solid var(--border);
      padding: 0.25rem 0.65rem;
      border-radius: 9999px;
      font-size: 0.75rem;
      font-weight: 500;
      cursor: pointer;
      transition: all 0.15s ease;
      user-select: none;
      display: inline-flex;
      align-items: center;
      gap: 0.35rem;
    }
    .pill:hover { background: var(--card-hover); color: var(--text); }
    .pill.active { background: var(--primary); color: #fff; border-color: var(--primary); font-weight: 600; }
    .pill.active .c-tag { background: rgba(255, 255, 255, 0.2); color: #fff; border-color: transparent; }
    .pill.active .pill-count { color: rgba(255, 255, 255, 0.8); }
    .pill-count { font-size: 0.7rem; font-family: monospace; opacity: 0.8; }
    .pill.active-warning { background: var(--warning-bg); color: var(--warning); border-color: var(--warning); font-weight: 600; }

    /* Table */
    .table-container {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 12px;
      overflow: hidden;
      box-shadow: var(--shadow);
      transition: background-color 0.2s ease, border-color 0.2s ease;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      text-align: left;
      font-size: 0.85rem;
    }
    thead {
      background: var(--card-subtle);
      border-bottom: 1px solid var(--border);
    }
    th {
      padding: 0.75rem 1rem;
      font-size: 0.725rem;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--text-muted);
      cursor: pointer;
      user-select: none;
      white-space: nowrap;
    }
    th:hover { color: var(--text); }
    tbody tr {
      border-bottom: 1px solid var(--border);
      transition: background-color 0.12s ease;
    }
    tbody tr:hover {
      background-color: var(--card-hover);
    }
    td {
      padding: 0.75rem 1rem;
      color: var(--text);
    }
    .node-name-cell {
      font-weight: 600;
      color: var(--text);
      max-width: 340px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 0.3rem;
      font-size: 0.725rem;
      font-weight: 600;
      padding: 0.2rem 0.5rem;
      border-radius: 6px;
    }
    .badge-proto { background: var(--primary-bg); color: var(--primary); }
    .badge-proto.hy2 { background: var(--purple-bg); color: var(--purple); }
    .badge-proto.vless { background: rgba(6, 182, 212, 0.15); color: #0891b2; }
    .badge-proto.trojan { background: var(--warning-bg); color: var(--warning); }
    .badge-loss { background: var(--danger-bg); color: var(--danger); border: 1px solid rgba(220, 38, 38, 0.3); cursor: pointer; }
    .badge-loss:hover { opacity: 0.85; }
    .badge-ok { background: var(--success-bg); color: var(--success); }
    .badge-blocked { background: var(--danger-bg); color: var(--danger); }
    .badge-unreachable { background: var(--card-subtle); color: var(--text-muted); border: 1px solid var(--border); }

    .btn-xs {
      padding: 0.25rem 0.55rem;
      font-size: 0.725rem;
      border-radius: 6px;
    }

    /* Modal / Drawer */
    .modal-backdrop {
      position: fixed;
      top: 0;
      left: 0;
      right: 0;
      bottom: 0;
      background: rgba(15, 23, 42, 0.6);
      backdrop-filter: blur(4px);
      display: none;
      align-items: center;
      justify-content: center;
      z-index: 100;
      padding: 1rem;
    }
    .modal-backdrop.active { display: flex; }
    .modal {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 16px;
      width: 100%;
      max-width: 720px;
      max-height: 90vh;
      overflow-y: auto;
      box-shadow: var(--shadow-lg);
      animation: modalSlide 0.2s ease-out;
    }
    @keyframes modalSlide {
      from { transform: scale(0.96) translateY(10px); opacity: 0; }
      to { transform: scale(1) translateY(0); opacity: 1; }
    }
    .modal-header {
      padding: 1.25rem 1.5rem;
      border-bottom: 1px solid var(--border);
      display: flex;
      align-items: center;
      justify-content: space-between;
    }
    .modal-title { font-size: 1.1rem; font-weight: 700; color: var(--text); }
    .modal-close {
      background: transparent;
      border: none;
      color: var(--text-muted);
      cursor: pointer;
      font-size: 1.25rem;
      line-height: 1;
      padding: 0.25rem;
      border-radius: 6px;
    }
    .modal-close:hover { color: var(--text); background: var(--card-hover); }
    .modal-body { padding: 1.5rem; }

    /* Timeline spark chart */
    .chart-box {
      background: var(--card-subtle);
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 1.25rem;
      margin-bottom: 1.25rem;
    }
    .chart-title { font-size: 0.85rem; font-weight: 600; color: var(--text-muted); margin-bottom: 0.75rem; }
    .chart-bars {
      display: flex;
      align-items: flex-end;
      gap: 8px;
      height: 100px;
      padding-top: 10px;
      border-bottom: 1px solid var(--border);
    }
    .chart-col {
      flex: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 4px;
      height: 100%;
      justify-content: flex-end;
    }
    .bar {
      width: 100%;
      max-width: 32px;
      border-radius: 4px 4px 0 0;
      background: var(--success);
      min-height: 4px;
      transition: height 0.3s ease;
    }
    .bar.timeout {
      background: repeating-linear-gradient(45deg, #ef4444, #ef4444 6px, #b91c1c 6px, #b91c1c 12px);
      height: 100% !important;
    }
    .bar-val { font-size: 0.7rem; font-weight: 600; color: var(--text-muted); }
    .bar-seq { font-size: 0.7rem; color: var(--text-muted); opacity: 0.7; }

    /* Alert callout */
    .callout-warning {
      background: var(--warning-bg);
      border: 1px solid rgba(217, 119, 6, 0.3);
      border-radius: 10px;
      padding: 0.9rem 1.1rem;
      margin-bottom: 1.25rem;
    }
    .callout-warning-title {
      font-size: 0.85rem;
      font-weight: 700;
      color: var(--warning);
      margin-bottom: 0.4rem;
      display: flex;
      align-items: center;
      gap: 0.4rem;
    }
    .callout-item {
      font-size: 0.8rem;
      color: var(--warning);
      font-family: monospace;
      margin-top: 0.25rem;
    }

    /* Samples list */
    .sample-row {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0.5rem 0.75rem;
      border-bottom: 1px solid var(--border);
      font-size: 0.825rem;
    }
    .sample-row:last-child { border-bottom: none; }
    .sample-time { color: var(--text-muted); font-family: monospace; }
    .empty-placeholder {
      text-align: center;
      padding: 3rem;
      color: var(--text-muted);
      font-size: 0.9rem;
    }
  </style>
</head>
<body>

  <!-- Report Data Injected -->
  <script id="report-data" type="application/json">` + safeJSON + `</script>

  <header>
    <div class="header-inner">
      <div class="brand">
        <div class="brand-icon">⚡</div>
        <div>
          <div style="display: flex; align-items: center; gap: 0.5rem;">
            <h1>Clash SpeedTest</h1>
            <span class="brand-tag">离线报告</span>
          </div>
          <p style="font-size: 0.75rem; color: var(--text-muted);" id="run-time-label">生成时间: -</p>
        </div>
      </div>
      <div class="header-actions">
        <select id="run-select" class="run-select" onchange="onSelectRun(this.value)">
          <!-- Run options populated dynamically -->
        </select>
        <button class="btn" onclick="toggleTheme()" id="theme-btn">
          <span id="theme-icon">🌙</span> <span id="theme-text">暗色</span>
        </button>
        <button class="btn" onclick="exportYAML()">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
          导出 Clash YAML
        </button>
        <button class="btn" onclick="exportCSV()">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
          导出 CSV
        </button>
      </div>
    </div>
  </header>

  <main class="container">
    <!-- Hero Stats -->
    <section class="stats-grid">
      <div class="stat-card c-blue">
        <span class="stat-title">测试节点数</span>
        <div class="stat-val" id="stat-total-nodes">0</div>
        <span class="stat-sub" id="stat-airport-name">机场: -</span>
      </div>
      <div class="stat-card c-green">
        <span class="stat-title">平均延迟</span>
        <div class="stat-val" id="stat-avg-latency">- ms</div>
        <span class="stat-sub" id="stat-latency-loss">丢包率: 0%</span>
      </div>
      <div class="stat-card c-purple">
        <span class="stat-title">平均下载带宽</span>
        <div class="stat-val" id="stat-avg-speed">- MB/s</div>
        <span class="stat-sub" id="stat-max-speed">最高: - MB/s</span>
      </div>
      <div class="stat-card c-amber">
        <span class="stat-title">AI & 服务解锁通过率</span>
        <div class="stat-val" id="stat-ag-rate">0%</div>
        <span class="stat-sub" id="stat-ag-count">可用节点: 0 / 0</span>
      </div>
    </section>

    <!-- Filter Card -->
    <section class="filter-card">
      <div class="filter-row">
        <span class="filter-label">搜索过滤</span>
        <input type="text" id="search-input" class="search-input" placeholder="按节点名称、IP 或出口地区搜索..." oninput="onFilterChange()">
        <button id="toggle-loss-btn" class="pill" onclick="toggleOnlyLoss()">
          ⚠️ 仅看有断连/超时的节点
        </button>
      </div>
      <div class="filter-row">
        <span class="filter-label">地区筛选</span>
        <div class="pill-group" id="country-pills"></div>
      </div>
      <div class="filter-row">
        <span class="filter-label">协议筛选</span>
        <div class="pill-group" id="proto-pills"></div>
      </div>
      <div class="filter-row">
        <span class="filter-label">服务解锁</span>
        <div class="pill-group" id="ag-pills"></div>
      </div>
    </section>

    <!-- Table -->
    <section class="table-container">
      <table>
        <thead>
          <tr>
            <th style="width: 50px;">#</th>
            <th onclick="sortTable('name')">节点名称 ↕</th>
            <th onclick="sortTable('proto')">协议 ↕</th>
            <th onclick="sortTable('country')">国家/地区 / IP ↕</th>
            <th onclick="sortTable('latency')">延迟 / 丢包 ↕</th>
            <th onclick="sortTable('jitter')">抖动 ↕</th>
            <th onclick="sortTable('speed')">下载速度 ↕</th>
            <th onclick="sortTable('ag')">服务解锁 ↕</th>
            <th style="text-align: right;">时序详情</th>
          </tr>
        </thead>
        <tbody id="nodes-tbody">
          <!-- Populated dynamically -->
        </tbody>
      </table>
      <div id="empty-state" class="empty-placeholder" style="display: none;">
        未找到符合筛选条件的节点
      </div>
    </section>
  </main>

  <!-- Modal for Latency Samples Timeline & Disconnection Inspector -->
  <div id="inspector-modal" class="modal-backdrop" onclick="closeInspectorOnBackdrop(event)">
    <div class="modal">
      <div class="modal-header">
        <div>
          <h3 class="modal-title" id="m-node-name">节点延迟采样时序</h3>
          <p style="font-size: 0.75rem; color: var(--text-muted);" id="m-node-meta">-</p>
        </div>
        <button class="modal-close" onclick="closeInspector()">&times;</button>
      </div>
      <div class="modal-body">
        <!-- Disconnection Callout if any timeouts occurred -->
        <div id="m-warning-box" class="callout-warning" style="display: none;">
          <div class="callout-warning-title">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
            <span id="m-warning-title">检测到断连或超时事件</span>
          </div>
          <div id="m-warning-items"></div>
        </div>

        <!-- Latency Bar Sparkline -->
        <div class="chart-box">
          <div class="chart-title">每次探测往返时延柱状分布 (毫秒)</div>
          <div class="chart-bars" id="m-chart-bars"></div>
        </div>

        <!-- Detailed Samples Table -->
        <div style="border: 1px solid var(--border); border-radius: 10px; overflow: hidden;">
          <div style="background: var(--card-subtle); padding: 0.6rem 0.75rem; font-size: 0.75rem; font-weight: 600; color: var(--text-muted); display: flex; justify-content: space-between; border-bottom: 1px solid var(--border);">
            <span>探测序列与时刻</span>
            <span>延迟 / 状态</span>
          </div>
          <div id="m-samples-list"></div>
        </div>
      </div>
    </div>
  </div>

  <script>
    const COUNTRY_MAP = {
      'HK': '香港', 'US': '美国', 'JP': '日本', 'SG': '新加坡',
      'TW': '台湾', 'KR': '韩国', 'UK': '英国', 'DE': '德国',
      'FR': '法国', 'CA': '加拿大', 'AU': '澳大利亚', 'RU': '俄罗斯',
      'IN': '印度', 'MY': '马来西亚', 'TH': '泰国', 'VN': '越南',
      'PH': '菲律宾', 'ID': '印尼', 'NL': '荷兰', 'TR': '土耳其',
      'BR': '巴西', 'AR': '阿根廷', 'ZA': '南非'
    };

    function getCountryLabel(code) {
      return COUNTRY_MAP[code] || code;
    }

    function getCountryBadgeClass(code) {
      const c = (code || '').toLowerCase();
      return ['hk','us','jp','sg','tw','kr','uk','de'].includes(c) ? 'c-tag-' + c : 'c-tag-other';
    }

    let allRuns = [];
    let currentRun = null;
    let filteredResults = [];
    let selectedCountry = 'ALL';
    let selectedProto = 'ALL';
    let selectedAG = 'ALL';
    let onlyLoss = false;
    let sortField = '';
    let sortAsc = true;
    let isDark = false;

    function initTheme() {
      const saved = localStorage.getItem('cs_theme');
      if (saved === 'dark') {
        setDarkTheme(true);
      }
    }

    function toggleTheme() {
      setDarkTheme(!isDark);
    }

    function setDarkTheme(dark) {
      isDark = dark;
      if (isDark) {
        document.body.classList.add('dark-theme');
        document.getElementById('theme-icon').textContent = '☀️';
        document.getElementById('theme-text').textContent = '亮色';
        localStorage.setItem('cs_theme', 'dark');
      } else {
        document.body.classList.remove('dark-theme');
        document.getElementById('theme-icon').textContent = '🌙';
        document.getElementById('theme-text').textContent = '暗色';
        localStorage.setItem('cs_theme', 'light');
      }
    }

    window.addEventListener('DOMContentLoaded', () => {
      initTheme();
      try {
        const raw = document.getElementById('report-data').textContent;
        allRuns = JSON.parse(raw) || [];
      } catch (e) {
        console.error('Failed to parse report data', e);
      }

      if (!allRuns || allRuns.length === 0) {
        document.getElementById('empty-state').style.display = 'block';
        return;
      }

      // Populate Run Select
      const runSelect = document.getElementById('run-select');
      runSelect.innerHTML = allRuns.map((r, idx) => {
        const dateStr = new Date(r.created_at).toLocaleString('zh-CN', { hour12: false });
        const name = r.airport_name || '机场测试';
        return '<option value="' + idx + '">' + dateStr + ' - ' + name + ' (' + (r.results ? r.results.length : 0) + ' 节点)</option>';
      }).join('');

      onSelectRun(0);
    });

    function onSelectRun(index) {
      currentRun = allRuns[index];
      if (!currentRun) return;

      const dateStr = new Date(currentRun.created_at).toLocaleString('zh-CN', { hour12: false });
      document.getElementById('run-time-label').textContent = '测试时间: ' + dateStr;
      document.getElementById('stat-airport-name').textContent = '机场: ' + (currentRun.airport_name || '自选');

      buildFilterPills();
      applyFilters();
    }

    function buildFilterPills() {
      const results = currentRun.results || [];
      const countries = new Map();
      const protos = new Set();
      const agStatuses = new Set();

      results.forEach(r => {
        const c = r.country_code || 'OTHER';
        countries.set(c, (countries.get(c) || 0) + 1);
        if (r.proxy_type) protos.add(r.proxy_type.toUpperCase());
        if (r.antigravity_status) agStatuses.add(r.antigravity_status);
      });

      // Country pills
      const cContainer = document.getElementById('country-pills');
      let cHTML = '<span class="pill ' + (selectedCountry === 'ALL' ? 'active' : '') + '" onclick="filterCountry(\'ALL\')">全部地区 <span class="pill-count">' + results.length + '</span></span>';
      countries.forEach((count, code) => {
        if (!code || code === 'OTHER') return;
        const active = selectedCountry === code ? 'active' : '';
        const label = getCountryLabel(code);
        const badgeClass = getCountryBadgeClass(code);
        cHTML += '<span class="pill ' + active + '" onclick="filterCountry(\'' + code + '\')">' +
          '<span class="c-tag ' + badgeClass + '" style="margin-right: 0.35rem; font-size: 0.65rem;">' + code + '</span>' +
          label + ' <span class="pill-count">' + count + '</span></span>';
      });
      cContainer.innerHTML = cHTML;

      // Proto pills
      const pContainer = document.getElementById('proto-pills');
      let pHTML = '<span class="pill ' + (selectedProto === 'ALL' ? 'active' : '') + '" onclick="filterProto(\'ALL\')">全部</span>';
      protos.forEach(p => {
        const active = selectedProto === p ? 'active' : '';
        pHTML += '<span class="pill ' + active + '" onclick="filterProto(\'' + p + '\')">' + p + '</span>';
      });
      pContainer.innerHTML = pHTML;

      // AG pills
      const agContainer = document.getElementById('ag-pills');
      let agHTML = '<span class="pill ' + (selectedAG === 'ALL' ? 'active' : '') + '" onclick="filterAG(\'ALL\')">全部</span>';
      agHTML += '<span class="pill ' + (selectedAG === 'available' ? 'active' : '') + '" onclick="filterAG(\'available\')">🟢 可用</span>';
      agHTML += '<span class="pill ' + (selectedAG === 'blocked' ? 'active' : '') + '" onclick="filterAG(\'blocked\')">🔴 地区不支持</span>';
      agHTML += '<span class="pill ' + (selectedAG === 'unreachable' ? 'active' : '') + '" onclick="filterAG(\'unreachable\')">⚪ 节点不通</span>';
      agContainer.innerHTML = agHTML;
    }

    function filterCountry(code) { selectedCountry = code; buildFilterPills(); applyFilters(); }
    function filterProto(proto) { selectedProto = proto; buildFilterPills(); applyFilters(); }
    function filterAG(status) { selectedAG = status; buildFilterPills(); applyFilters(); }
    function toggleOnlyLoss() {
      onlyLoss = !onlyLoss;
      const btn = document.getElementById('toggle-loss-btn');
      if (onlyLoss) {
        btn.classList.add('active-warning');
      } else {
        btn.classList.remove('active-warning');
      }
      applyFilters();
    }
    function onFilterChange() { applyFilters(); }

    function sortTable(field) {
      if (sortField === field) {
        sortAsc = !sortAsc;
      } else {
        sortField = field;
        sortAsc = true;
      }

      filteredResults.sort((a, b) => {
        let valA = a[field];
        let valB = b[field];
        if (field === 'name') { valA = a.proxy_name || ''; valB = b.proxy_name || ''; }
        else if (field === 'proto') { valA = a.proxy_type || ''; valB = b.proxy_type || ''; }
        else if (field === 'country') { valA = a.country_code || ''; valB = b.country_code || ''; }
        else if (field === 'latency') { valA = a.latency_ms || 999999; valB = b.latency_ms || 999999; }
        else if (field === 'jitter') { valA = a.jitter_ms || 999999; valB = b.jitter_ms || 999999; }
        else if (field === 'speed') { valA = a.download_speed_mbps || 0; valB = b.download_speed_mbps || 0; }
        else if (field === 'ag') { valA = a.antigravity_status || ''; valB = b.antigravity_status || ''; }

        if (valA < valB) return sortAsc ? -1 : 1;
        if (valA > valB) return sortAsc ? 1 : -1;
        return 0;
      });

      renderTable();
    }

    function applyFilters() {
      if (!currentRun || !currentRun.results) return;
      const q = (document.getElementById('search-input').value || '').trim().toLowerCase();

      filteredResults = currentRun.results.filter(r => {
        if (q) {
          const matchName = (r.proxy_name || '').toLowerCase().includes(q);
          const matchCountry = (r.country_code || '').toLowerCase().includes(q) || (r.exit_country || '').toLowerCase().includes(q);
          const matchServer = (r.server || '').toLowerCase().includes(q);
          if (!matchName && !matchCountry && !matchServer) return false;
        }
        if (selectedCountry !== 'ALL' && (r.country_code || 'OTHER') !== selectedCountry) return false;
        if (selectedProto !== 'ALL' && (r.proxy_type || '').toUpperCase() !== selectedProto) return false;
        if (selectedAG !== 'ALL' && r.antigravity_status !== selectedAG) return false;
        if (onlyLoss) {
          const hasLoss = (r.packet_loss && r.packet_loss > 0) || (r.timeout_count && r.timeout_count > 0);
          if (!hasLoss) return false;
        }
        return true;
      });

      renderKPIs();
      renderTable();
    }

    function renderKPIs() {
      const results = filteredResults;
      document.getElementById('stat-total-nodes').textContent = results.length;

      let sumLat = 0, countLat = 0, totalLossSum = 0, sumSpeed = 0, countSpeed = 0, maxSpeed = 0, agPass = 0, agTested = 0;

      results.forEach(r => {
        if (r.latency_ms > 0) {
          sumLat += r.latency_ms;
          countLat++;
        }
        totalLossSum += (r.packet_loss || 0);
        if (r.download_speed_mbps > 0) {
          sumSpeed += r.download_speed_mbps;
          countSpeed++;
          if (r.download_speed_mbps > maxSpeed) maxSpeed = r.download_speed_mbps;
        }
        if (r.antigravity_status) {
          agTested++;
          if (r.antigravity_status === 'available') agPass++;
        }
      });

      const avgLat = countLat > 0 ? Math.round(sumLat / countLat) : '-';
      document.getElementById('stat-avg-latency').textContent = avgLat !== '-' ? avgLat + ' ms' : '-';
      const avgLoss = results.length > 0 ? (totalLossSum / results.length).toFixed(1) : '0';
      document.getElementById('stat-latency-loss').textContent = '平均丢包率: ' + avgLoss + '%';

      const avgSpeed = countSpeed > 0 ? (sumSpeed / countSpeed).toFixed(2) : '-';
      document.getElementById('stat-avg-speed').textContent = avgSpeed !== '-' ? avgSpeed + ' MB/s' : '-';
      document.getElementById('stat-max-speed').textContent = '峰值: ' + maxSpeed.toFixed(2) + ' MB/s';

      const agRate = agTested > 0 ? Math.round((agPass / agTested) * 100) : 0;
      document.getElementById('stat-ag-rate').textContent = agRate + '%';
      document.getElementById('stat-ag-count').textContent = '绿灯可用: ' + agPass + ' / ' + agTested;
    }

    function renderTable() {
      const tbody = document.getElementById('nodes-tbody');
      const empty = document.getElementById('empty-state');

      if (filteredResults.length === 0) {
        tbody.innerHTML = '';
        empty.style.display = 'block';
        return;
      }
      empty.style.display = 'none';

      tbody.innerHTML = filteredResults.map((r, i) => {
        const proto = (r.proxy_type || '').toLowerCase();
        let protoClass = 'badge-proto';
        if (proto.includes('hy2') || proto.includes('hysteria')) protoClass += ' hy2';
        else if (proto.includes('vless')) protoClass += ' vless';
        else if (proto.includes('trojan')) protoClass += ' trojan';

        let latHTML = '<span style="color: var(--text-muted);">-</span>';
        if (r.latency_ms > 0) {
          latHTML = '<span style="font-weight: 600;">' + r.latency_ms + 'ms</span>';
          if (r.packet_loss > 0 || r.timeout_count > 0) {
            latHTML += ' <span class="badge badge-loss" onclick="openInspector(' + i + ')" title="点击查看具体断连超时时刻">⚠️ ' + (r.timeout_count || 1) + '次断连 (' + r.packet_loss.toFixed(0) + '%)</span>';
          }
        } else if (r.packet_loss === 100) {
          latHTML = '<span class="badge badge-loss" onclick="openInspector(' + i + ')">100% 丢包</span>';
        }

        let speedHTML = '<span style="color: var(--text-muted);">-</span>';
        if (r.download_speed_mbps > 0) {
          speedHTML = '<span style="font-weight: 600; color: var(--success);">' + r.download_speed_mbps.toFixed(2) + ' MB/s</span>';
        } else if (r.download_error) {
          speedHTML = '<span style="color: var(--danger); font-size: 0.75rem;">' + escapeHTML(r.download_error) + '</span>';
        }

        let agHTML = '<span style="color: var(--text-muted);">-</span>';
        if (r.antigravity_status === 'available') {
          agHTML = '<span class="badge badge-ok">🟢 官方可用</span>';
        } else if (r.antigravity_status === 'flapping') {
          agHTML = '<span class="badge badge-unreachable" style="background: rgba(245,158,11,0.18); color: #d97706; font-weight: 600;">🟡 偶发送中</span>';
        } else if (r.antigravity_status === 'blocked') {
          agHTML = '<span class="badge badge-blocked">🔴 地区不支持</span>';
        } else if (r.antigravity_status === 'unreachable') {
          agHTML = '<span class="badge badge-unreachable">⚪ 节点不通</span>';
        } else if (r.antigravity_status === 'auth_failed') {
          agHTML = '<span class="badge badge-unreachable" style="background: rgba(245,158,11,0.15); color: #f59e0b;">🟡 凭据失效</span>';
        }
        if (r.google_ttfb_ms > 0) {
          agHTML += ' <span class="badge" style="font-size: 0.68rem; font-family: monospace; background: rgba(59,130,246,0.12); color: #2563eb;">⚡ ' + r.google_ttfb_ms + 'ms</span>';
        }
        if (r.stability && r.stability.blocked_ips && r.stability.blocked_ips.length > 0) {
          agHTML += '<div style="font-size: 0.68rem; color: #dc2626; font-family: monospace; margin-top: 2px;">受阻出口: ' + escapeHTML(r.stability.blocked_ips.join(', ')) + '</div>';
        }
        if (r.antigravity_detail) {
          agHTML += '<div style="font-size: 0.68rem; color: var(--text-muted); margin-top: 2px;" title="' + escapeHTML(r.antigravity_detail) + '">' + escapeHTML(r.antigravity_detail) + '</div>';
        }

        const countryCode = r.country_code || '';
        let countryHTML = '<span style="color: var(--text-muted);">-</span>';
        if (countryCode && countryCode !== 'OTHER') {
          const countryLabel = getCountryLabel(countryCode);
          const badgeClass = getCountryBadgeClass(countryCode);
          countryHTML = '<div style="display: flex; align-items: center; gap: 0.4rem;">' +
            '<span class="c-tag ' + badgeClass + '">' + countryCode + '</span>' +
            '<span style="font-weight: 500; font-size: 0.8rem;">' + countryLabel + '</span>' +
            '</div>';
        }
        if (r.ip_info && r.ip_info.ip) {
          const typeBadge = r.ip_info.ip_type === '住宅家宽' ? '🏠 住宅' : '🏢 机房';
          const originBadge = r.ip_info.origin_type === '原生IP' ? '🌱 原生' : (r.ip_info.origin_type === '广播IP' ? '📡 广播' : '');
          let scoreParts = [];
          if (r.ip_info.risk_score !== undefined && r.ip_info.risk_score !== null) scoreParts.push('Ping0: ' + r.ip_info.risk_score + '%');
          if (r.ip_info.fraud_score !== undefined && r.ip_info.fraud_score !== null && r.ip_info.fraud_score > 0) scoreParts.push('IPPure: ' + r.ip_info.fraud_score + '%');
          const badges = [typeBadge, originBadge, scoreParts.join(' | ')].filter(Boolean).join(' · ');
          countryHTML += '<div style="font-size: 0.68rem; color: var(--text-muted); margin-top: 3px; font-family: monospace;" title="' + escapeHTML((r.ip_info.isp || '') + ' | ' + (r.ip_info.asn || '')) + '">' +
            escapeHTML(r.ip_info.ip) + '<br>' + badges +
            '</div>';
        }

        return '<tr>' +
          '<td style="color: var(--text-muted); font-size: 0.75rem;">' + (i + 1) + '</td>' +
          '<td><div class="node-name-cell" title="' + escapeHTML(r.proxy_name) + '">' + escapeHTML(r.proxy_name) + '</div></td>' +
          '<td><span class="badge ' + protoClass + '">' + (r.proxy_type || '-') + '</span></td>' +
          '<td>' + countryHTML + '</td>' +
          '<td>' + latHTML + '</td>' +
          '<td>' + (r.jitter_ms ? r.jitter_ms + 'ms' : '-') + '</td>' +
          '<td>' + speedHTML + '</td>' +
          '<td>' + agHTML + '</td>' +
          '<td style="text-align: right;">' +
            '<button class="btn btn-xs" onclick="openInspector(' + i + ')">📊 采样时序</button>' +
          '</td>' +
        '</tr>';
      }).join('');
    }

    // Inspector
    function openInspector(index) {
      const r = filteredResults[index];
      if (!r) return;

      document.getElementById('m-node-name').textContent = r.proxy_name;
      document.getElementById('m-node-meta').textContent = (r.proxy_type || '') + ' | ' + (r.country_code || '') + ' | 延迟: ' + (r.latency_ms || 0) + 'ms | 丢包: ' + (r.packet_loss || 0) + '%';

      const samples = r.latency_samples || [];
      const warningBox = document.getElementById('m-warning-box');
      const warningItems = document.getElementById('m-warning-items');
      const chartBars = document.getElementById('m-chart-bars');
      const samplesList = document.getElementById('m-samples-list');

      // Check for timeouts / errors
      const timeoutSamples = samples.filter(s => !s.success);
      if (timeoutSamples.length > 0) {
        warningBox.style.display = 'block';
        document.getElementById('m-warning-title').textContent = '检测到 ' + timeoutSamples.length + ' 次断连或超时事件';
        warningItems.innerHTML = timeoutSamples.map(ts => {
          const tStr = ts.timestamp ? new Date(ts.timestamp).toLocaleTimeString('zh-CN', { hour12: false, fractionalSecondDigits: 3 }) : '时刻未知';
          return '<div class="callout-item">⚠️ ' + tStr + ' - 第 ' + ts.seq + ' 次探测断连: ' + escapeHTML(ts.error || '连接超时') + '</div>';
        }).join('');
      } else {
        warningBox.style.display = 'none';
      }

      // Render Bar Chart
      if (samples.length > 0) {
        let maxLat = Math.max(...samples.map(s => s.latency_ms || 0), 100);
        chartBars.innerHTML = samples.map(s => {
          if (!s.success) {
            return '<div class="chart-col">' +
              '<div class="bar-val" style="color: var(--danger);">超时</div>' +
              '<div class="bar timeout"></div>' +
              '<div class="bar-seq">#' + s.seq + '</div>' +
            '</div>';
          }
          const heightPct = Math.max(8, Math.min(100, Math.round((s.latency_ms / maxLat) * 90)));
          return '<div class="chart-col">' +
            '<div class="bar-val">' + s.latency_ms + 'ms</div>' +
            '<div class="bar" style="height: ' + heightPct + '%;"></div>' +
            '<div class="bar-seq">#' + s.seq + '</div>' +
          '</div>';
        }).join('');

        // Render Samples List
        samplesList.innerHTML = samples.map(s => {
          const tStr = s.timestamp ? new Date(s.timestamp).toLocaleTimeString('zh-CN', { hour12: false, fractionalSecondDigits: 3 }) : '-';
          if (!s.success) {
            return '<div class="sample-row" style="background: var(--danger-bg);">' +
              '<div><span class="sample-time">' + tStr + '</span> <span style="font-weight: 600; margin-left: 0.5rem;">第 ' + s.seq + ' 次</span></div>' +
              '<div style="color: var(--danger); font-weight: 600;">⚠️ 断连超时 (' + escapeHTML(s.error || '超时') + ')</div>' +
            '</div>';
          }
          return '<div class="sample-row">' +
            '<div><span class="sample-time">' + tStr + '</span> <span style="font-weight: 600; margin-left: 0.5rem;">第 ' + s.seq + ' 次</span></div>' +
            '<div style="color: var(--success); font-weight: 600;">' + s.latency_ms + ' ms</div>' +
          '</div>';
        }).join('');
      } else {
        chartBars.innerHTML = '<div style="color: var(--text-muted); font-size: 0.8rem; padding: 1rem;">未记录单独采样（仅有均值统计）</div>';
        samplesList.innerHTML = '<div style="color: var(--text-muted); font-size: 0.8rem; padding: 1rem; text-align: center;">无逐次采样历史</div>';
      }

      document.getElementById('inspector-modal').classList.add('active');
    }

    function closeInspector() {
      document.getElementById('inspector-modal').classList.remove('active');
    }
    function closeInspectorOnBackdrop(e) {
      if (e.target.id === 'inspector-modal') closeInspector();
    }

    function escapeHTML(str) {
      if (!str) return '';
      return String(str).replace(/[&<>"']/g, function(m) {
        return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m];
      });
    }

    // Client-side Export
    function exportYAML() {
      if (!filteredResults || filteredResults.length === 0) return alert('当前没有选中的节点可导出');
      const lines = ['proxies:'];
      filteredResults.forEach(r => {
        lines.push('  - name: "' + (r.proxy_name || '').replace(/"/g, '\\"') + '"');
        lines.push('    type: ' + (r.proxy_type || 'ss'));
        lines.push('    server: ' + (r.server || '127.0.0.1'));
        lines.push('    port: ' + (r.port || 443));
      });
      downloadFile('clash_export.yaml', lines.join('\n'), 'text/yaml');
    }

    function exportCSV() {
      if (!filteredResults || filteredResults.length === 0) return alert('当前没有节点可导出');
      let csv = '\uFEFF序号,节点名称,协议,国家地区,延迟(ms),抖动(ms),丢包率(%),下载速度(MB/s),断连次数,Antigravity\n';
      filteredResults.forEach((r, idx) => {
        csv += (idx + 1) + ',"' + (r.proxy_name || '').replace(/"/g, '""') + '",' +
          (r.proxy_type || '') + ',' +
          (r.country_code || '') + ',' +
          (r.latency_ms || '') + ',' +
          (r.jitter_ms || '') + ',' +
          (r.packet_loss || 0) + ',' +
          (r.download_speed_mbps || 0) + ',' +
          (r.timeout_count || 0) + ',' +
          (r.antigravity_status || '') + '\n';
      });
      downloadFile('speedtest_export.csv', csv, 'text/csv;charset=utf-8;');
    }

    function downloadFile(filename, content, mime) {
      const blob = new Blob([content], { type: mime });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    }
  </script>
</body>
</html>`

	return tmpl, nil
}
