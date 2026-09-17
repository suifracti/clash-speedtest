package profiles

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/speedtester"
	"golang.org/x/term"
)

type App struct {
	Paths Paths
	In    *bufio.Reader
	Out   io.Writer
	UA    string
}

type Selection struct {
	Airport    *Airport
	ConfigPath string
}

type TestPlan struct {
	Metrics  speedtester.MetricSet
	Duration time.Duration
	Rounds   int
}

func NewApp(userAgent string) *App {
	return &App{
		Paths: DefaultPaths(),
		In:    bufio.NewReader(os.Stdin),
		Out:   os.Stdout,
		UA:    userAgent,
	}
}

func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func (a *App) printf(format string, args ...any) {
	fmt.Fprintf(a.Out, format, args...)
}

func (a *App) readLine(prompt string) (string, error) {
	a.printf("%s", prompt)
	line, err := a.In.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	return strings.TrimSpace(line), nil
}

func (a *App) confirm(prompt string, defaultYes bool) (bool, error) {
	hint := "[Y/n] "
	if !defaultYes {
		hint = "[y/N] "
	}
	line, err := a.readLine(prompt + hint)
	if err != nil {
		return false, err
	}
	if line == "" {
		return defaultYes, nil
	}
	switch strings.ToLower(line) {
	case "y", "yes", "是":
		return true, nil
	case "n", "no", "否":
		return false, nil
	default:
		return defaultYes, nil
	}
}

func (a *App) SelectAirport() (*Selection, error) {
	store, err := LoadStore(a.Paths.StoreFile())
	if err != nil {
		return nil, err
	}
	if a.Paths.ImportLegacyIfEmpty(store) {
		if err := SaveStore(a.Paths.StoreFile(), store); err != nil {
			return nil, err
		}
		a.printf("已从 run.local.env 导入 1 个机场。\n")
	}

	for {
		a.printAirportMenu(store)
		line, err := a.readLine("请选择: ")
		if err != nil {
			return nil, err
		}
		if line == "" {
			continue
		}
		switch strings.ToUpper(line) {
		case "A", "ADD", "添加":
			if err := a.addAirport(store); err != nil {
				return nil, err
			}
			continue
		case "E", "U", "R", "EDIT", "UPDATE", "REPLACE", "更换", "修改":
			if err := a.editAirport(store); err != nil {
				return nil, err
			}
			continue
		case "D", "DEL", "DELETE", "删除":
			if err := a.deleteAirport(store); err != nil {
				return nil, err
			}
			continue
		case "Q", "QUIT", "EXIT", "退出":
			return nil, nil
		}

		index, convErr := strconv.Atoi(line)
		if convErr != nil || index < 1 || index > len(store.Airports) {
			a.printf("无效选择，请输入编号，或 A/E/D/Q。\n")
			continue
		}
		airport := store.Airports[index-1]
		configPath, err := a.prepareConfig(store, airport)
		if err != nil {
			a.printf("准备订阅失败: %s\n", err)
			continue
		}
		return &Selection{Airport: airport, ConfigPath: configPath}, nil
	}
}

func (a *App) printAirportMenu(store *Store) {
	a.printf("\n========================================\n")
	a.printf(" Clash SpeedTest  机场管理\n")
	a.printf("========================================\n\n")
	if len(store.Airports) == 0 {
		a.printf("  （还没有保存的机场）\n\n")
	} else {
		for i, airport := range store.Airports {
			updated := "尚未下载"
			if !airport.UpdatedAt.IsZero() {
				updated = "上次更新 " + airport.UpdatedAt.Local().Format("01-02 15:04")
			} else if a.Paths.HasCache(airport.ID) {
				updated = "已有缓存"
			}
			a.printf("  %d) %s    %s\n", i+1, airport.Name, updated)
			a.printf("     %s\n", RedactURL(airport.URL))
		}
		a.printf("\n")
	}
	a.printf("  A) 添加机场\n")
	a.printf("  E) 更换订阅地址\n")
	a.printf("  D) 删除机场\n")
	a.printf("  Q) 退出\n\n")
}

func (a *App) addAirport(store *Store) error {
	name, err := a.readLine("机场名称: ")
	if err != nil {
		return err
	}
	url, err := a.readLine("订阅地址或本地 config.yaml 路径: ")
	if err != nil {
		return err
	}
	url = strings.Trim(url, `"'`)
	url = ExpandLocalPath(url)
	if url == "" {
		a.printf("地址不能为空。\n")
		return nil
	}
	if name == "" {
		name = defaultAirportName(url)
	}
	airport := &Airport{
		ID:   newAirportID(),
		Name: name,
		URL:  url,
	}
	store.Add(airport)
	if err := SaveStore(a.Paths.StoreFile(), store); err != nil {
		return err
	}
	a.printf("已添加「%s」。\n", name)
	return nil
}

func (a *App) editAirport(store *Store) error {
	airport, err := a.pickAirport(store, "要更换订阅的编号: ")
	if err != nil {
		return err
	}
	if airport == nil {
		return nil
	}

	a.printf("当前机场: %s\n", airport.Name)
	a.printf("当前地址: %s\n", RedactURL(airport.URL))
	name, err := a.readLine("新名称（回车保持不变）: ")
	if err != nil {
		return err
	}
	url, err := a.readLine("新订阅地址或本地 config.yaml 路径: ")
	if err != nil {
		return err
	}
	url = strings.Trim(url, `"'`)
	url = ExpandLocalPath(url)
	if url == "" {
		a.printf("地址不能为空。\n")
		return nil
	}

	if applyAirportEdit(airport, name, url) {
		a.Paths.RemoveCache(airport.ID)
	}
	if err := SaveStore(a.Paths.StoreFile(), store); err != nil {
		return err
	}
	a.printf("已更新「%s」。下次选择该机场时会用新地址下载。\n", airport.Name)
	return nil
}

func (a *App) pickAirport(store *Store, prompt string) (*Airport, error) {
	if len(store.Airports) == 0 {
		a.printf("还没有保存的机场。\n")
		return nil, nil
	}
	line, err := a.readLine(prompt)
	if err != nil {
		return nil, err
	}
	index, convErr := strconv.Atoi(line)
	if convErr != nil || index < 1 || index > len(store.Airports) {
		a.printf("编号无效。\n")
		return nil, nil
	}
	return store.Airports[index-1], nil
}

func (a *App) deleteAirport(store *Store) error {
	airport, err := a.pickAirport(store, "要删除的编号: ")
	if err != nil {
		return err
	}
	if airport == nil {
		return nil
	}
	ok, err := a.confirm(fmt.Sprintf("确认删除「%s」？", airport.Name), false)
	if err != nil {
		return err
	}
	if !ok {
		a.printf("已取消。\n")
		return nil
	}
	store.Remove(airport.ID)
	a.Paths.RemoveCache(airport.ID)
	if err := SaveStore(a.Paths.StoreFile(), store); err != nil {
		return err
	}
	a.printf("已删除「%s」。\n", airport.Name)
	return nil
}

func (a *App) prepareConfig(store *Store, airport *Airport) (string, error) {
	if !IsHTTPURL(airport.URL) {
		path := ExpandLocalPath(airport.URL)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("本地配置不存在: %s", path)
		}
		return path, nil
	}

	hasCache := a.Paths.HasCache(airport.ID)
	shouldUpdate := true
	if hasCache {
		a.printf("\n机场: %s\n", airport.Name)
		ok, err := a.confirm("是否更新订阅（重新下载最新节点）？", true)
		if err != nil {
			return "", err
		}
		shouldUpdate = ok
	}

	if !shouldUpdate && hasCache {
		a.printf("使用本地缓存。\n")
		return a.Paths.CacheFile(airport.ID), nil
	}

	a.printf("正在下载订阅...\n")
	body, err := FetchSubscription(airport.URL, a.UA)
	if err != nil {
		if hasCache {
			a.printf("更新失败: %s\n", err)
			useCache, confErr := a.confirm("改用上次缓存？", true)
			if confErr != nil {
				return "", confErr
			}
			if useCache {
				return a.Paths.CacheFile(airport.ID), nil
			}
		}
		return "", err
	}
	if err := a.Paths.WriteCache(airport.ID, body); err != nil {
		return "", err
	}
	airport.UpdatedAt = time.Now()
	if err := SaveStore(a.Paths.StoreFile(), store); err != nil {
		return "", err
	}
	a.printf("订阅已更新。\n")
	return a.Paths.CacheFile(airport.ID), nil
}

func (a *App) SelectCountries(names []string) (selected []string, all bool, err error) {
	if len(names) == 0 {
		return nil, true, nil
	}
	groups := GroupByCountry(names)
	for {
		a.printf("\n读取到 %d 个节点:\n\n", len(names))
		for i, group := range groups {
			a.printf("  %d) %s %s    %d\n", i+1, group.Flag, group.Label, len(group.Names))
		}
		a.printf("  A) 全部\n\n")
		line, err := a.readLine("请选择要测的国家（多选，逗号分隔，如 1,3 或 香港,日本）: ")
		if err != nil {
			return nil, false, err
		}
		all, codes, parseErr := parseCountrySelection(line, groups)
		if parseErr != nil {
			a.printf("%s\n", parseErr)
			continue
		}
		if all {
			return names, true, nil
		}
		allow := make(map[string]struct{}, len(codes))
		for _, code := range codes {
			allow[code] = struct{}{}
		}
		picked := make([]string, 0)
		for _, group := range groups {
			if _, ok := allow[group.Code]; ok {
				picked = append(picked, group.Names...)
			}
		}
		if len(picked) == 0 {
			a.printf("没有匹配的节点，请重新选择。\n")
			continue
		}
		a.printf("将测试 %d 个节点。\n", len(picked))
		return picked, false, nil
	}
}

func (a *App) SelectMetrics() (speedtester.MetricSet, error) {
	return a.selectMetrics()
}

func (a *App) SelectDuration() (time.Duration, error) {
	d, _, err := a.selectPlan(speedtester.MetricSet{})
	return d, err
}

func (a *App) SelectPlan(metrics speedtester.MetricSet) (time.Duration, int, error) {
	return a.selectPlan(metrics)
}

func (a *App) SelectTestPlan() (TestPlan, error) {
	metrics, err := a.selectMetrics()
	if err != nil {
		return TestPlan{}, err
	}
	duration, rounds, err := a.selectPlan(metrics)
	if err != nil {
		return TestPlan{}, err
	}
	return TestPlan{Metrics: metrics, Duration: duration, Rounds: rounds}, nil
}

func (a *App) selectMetrics() (speedtester.MetricSet, error) {
	for {
		a.printf("\n请选择测试项目（可多选，逗号分隔）:\n\n")
		a.printf("  1) 真实延迟\n")
		a.printf("  2) 下载速度\n")
		a.printf("  3) 上传速度\n")
		a.printf("  4) Antigravity 地区\n")
		a.printf("  A) 全部\n\n")
		line, err := a.readLine("请选择: ")
		if err != nil {
			return speedtester.MetricSet{}, err
		}
		metrics, parseErr := parseMetricSelection(line)
		if parseErr != nil {
			a.printf("%s\n", parseErr)
			continue
		}
		return metrics, nil
	}
}

func (a *App) selectPlan(metrics speedtester.MetricSet) (time.Duration, int, error) {
	isLatencyOnly := metrics.IsLatencyOnly()
	for {
		if isLatencyOnly {
			a.printf("\n当前为纯延迟测试（流量极低），可选择是否持续监控线路稳定性:\n\n")
			a.printf("  1) 快速测一轮（默认，直接按回车）\n")
			a.printf("  2) 持续监控 3 分钟（观察丢包与抖动）\n")
			a.printf("  3) 持续监控 5 分钟\n")
			a.printf("  4) 持续监控 10 分钟\n")
			a.printf("  也可输入具体时长（如 8m）或轮数（如 2l）\n\n")
		} else {
			a.printf("\n当前包含带宽测速（消耗流量），建议选择测试轮数:\n\n")
			a.printf("  1) 测一轮（默认，直接按回车）\n")
			a.printf("  2) 测 2 轮（输入 2 或 2l）\n")
			a.printf("  3) 测 3 轮（输入 3 或 3l）\n")
			a.printf("  也可输入更多轮数（如 5l），或时长（如 5m）\n\n")
		}
		line, err := a.readLine("请选择 [1]: ")
		if err != nil {
			return 0, 0, err
		}
		duration, rounds, parseErr := parsePlanSelection(line, isLatencyOnly)
		if parseErr != nil {
			a.printf("%s\n", parseErr)
			continue
		}
		return duration, rounds, nil
	}
}

func parseMetricSelection(input string) (speedtester.MetricSet, error) {
	input = strings.TrimSpace(input)
	if input == "" || strings.EqualFold(input, "a") || strings.EqualFold(input, "all") || input == "全部" {
		return speedtester.MetricSet{Latency: true, Download: true, Upload: true, Antigravity: true}, nil
	}
	var metrics speedtester.MetricSet
	for _, part := range splitSelection(input) {
		if n, err := strconv.Atoi(part); err == nil {
			switch n {
			case 1:
				metrics.Latency = true
			case 2:
				metrics.Download = true
			case 3:
				metrics.Upload = true
			case 4:
				metrics.Antigravity = true
			default:
				return speedtester.MetricSet{}, fmt.Errorf("编号 %d 超出范围", n)
			}
			continue
		}
		parsed, err := speedtester.ParseMetrics(part)
		if err != nil {
			return speedtester.MetricSet{}, fmt.Errorf("无法识别 %q", part)
		}
		metrics.Latency = metrics.Latency || parsed.Latency
		metrics.Download = metrics.Download || parsed.Download
		metrics.Upload = metrics.Upload || parsed.Upload
		metrics.Antigravity = metrics.Antigravity || parsed.Antigravity
	}
	if metrics.IsZero() {
		return speedtester.MetricSet{}, fmt.Errorf("请至少选择一个测试项目")
	}
	return metrics, nil
}

func parsePlanSelection(input string, isLatencyOnly bool) (time.Duration, int, error) {
	input = strings.TrimSpace(input)
	if input == "" || input == "1" || strings.EqualFold(input, "1l") || strings.EqualFold(input, "1r") || input == "1轮" {
		return 0, 1, nil
	}

	lower := strings.ToLower(input)

	// Check round suffixes: 2l, 3l, 2r, 2轮, etc.
	if strings.HasSuffix(lower, "l") || strings.HasSuffix(lower, "r") || strings.HasSuffix(lower, "轮") {
		s := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(lower, "l"), "r"), "轮")
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return 0, 0, fmt.Errorf("无法识别轮数 %q", input)
		}
		return 0, n, nil
	}

	// Check duration suffixes: 3m, 5m, 1h, 30s, 5分钟, etc.
	lowerDuration := strings.TrimSuffix(lower, "钟")
	lowerDuration = strings.ReplaceAll(lowerDuration, "分钟", "m")
	lowerDuration = strings.ReplaceAll(lowerDuration, "分", "m")
	if strings.HasSuffix(lowerDuration, "m") || strings.HasSuffix(lowerDuration, "h") || strings.HasSuffix(lowerDuration, "s") {
		d, err := time.ParseDuration(lowerDuration)
		if err != nil || d <= 0 {
			return 0, 0, fmt.Errorf("无法识别时长 %q", input)
		}
		return d, 0, nil
	}

	// Pure integer input
	n, err := strconv.Atoi(lower)
	if err != nil || n <= 0 {
		return 0, 0, fmt.Errorf("无法识别选项 %q", input)
	}

	if isLatencyOnly {
		switch n {
		case 1:
			return 0, 1, nil
		case 2:
			return 3 * time.Minute, 0, nil
		case 3:
			return 5 * time.Minute, 0, nil
		case 4:
			return 10 * time.Minute, 0, nil
		case 5:
			return 15 * time.Minute, 0, nil
		case 6:
			return 30 * time.Minute, 0, nil
		default:
			// e.g. 8 -> 8 minutes
			return time.Duration(n) * time.Minute, 0, nil
		}
	}

	// In bandwidth mode, a pure number represents rounds! e.g. "2" -> 2 rounds
	return 0, n, nil
}

func parseDurationSelection(input string) (time.Duration, error) {
	d, _, err := parsePlanSelection(input, true)
	return d, err
}

func parseCountrySelection(input string, groups []CountryGroup) (all bool, codes []string, err error) {
	input = strings.TrimSpace(input)
	if input == "" || strings.EqualFold(input, "a") || strings.EqualFold(input, "all") || input == "全部" {
		return true, nil, nil
	}
	parts := splitSelection(input)
	seen := make(map[string]struct{})
	for _, part := range parts {
		if n, convErr := strconv.Atoi(part); convErr == nil {
			if n < 1 || n > len(groups) {
				return false, nil, fmt.Errorf("编号 %d 超出范围", n)
			}
			code := groups[n-1].Code
			if _, ok := seen[code]; !ok {
				seen[code] = struct{}{}
				codes = append(codes, code)
			}
			continue
		}
		code, ok := LookupCountryCode(part)
		if !ok {
			for _, group := range groups {
				if strings.EqualFold(part, group.Label) || strings.EqualFold(part, group.Code) {
					code = group.Code
					ok = true
					break
				}
			}
		}
		if !ok {
			return false, nil, fmt.Errorf("无法识别 %q", part)
		}
		if _, exists := seen[code]; !exists {
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return false, nil, fmt.Errorf("请至少选择一个国家")
	}
	return false, codes, nil
}

func splitSelection(input string) []string {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '、' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func defaultAirportName(rawURL string) string {
	if !IsHTTPURL(rawURL) {
		base := rawURL
		if i := strings.LastIndexAny(base, `/\`); i >= 0 {
			base = base[i+1:]
		}
		if base == "" {
			return "本地配置"
		}
		return base
	}
	return "未命名机场"
}
