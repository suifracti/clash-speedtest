package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/faceair/clash-speedtest/gist"
	"github.com/faceair/clash-speedtest/gui"
	"github.com/faceair/clash-speedtest/ip"
	"github.com/faceair/clash-speedtest/output"
	"github.com/faceair/clash-speedtest/profiles"
	"github.com/faceair/clash-speedtest/speedtester"
	"github.com/faceair/clash-speedtest/tui"
	mihomolog "github.com/metacubex/mihomo/log"
	"gopkg.in/yaml.v2"
)

// Version information injected via ldflags during build
var (
	version = "dev"
	commit  = "unknown"
)

var (
	configPathsConfig    = flag.String("c", "", "config file path, also support http(s) url")
	filterRegexConfig    = flag.String("f", ".+", "filter proxies by name, use regexp")
	blockKeywords        = flag.String("b", "", "block proxies by keywords, use | to separate multiple keywords (example: -b 'rate|x1|1x')")
	serverURL            = flag.String("server-url", "https://dl.google.com/chrome/mac/universal/stable/GGRO/googlechrome.dmg", "server url or direct download url")
	speedMode            = flag.String("speed-mode", "download", "speed test mode: fast, download, full")
	downloadSize         = flag.Int("download-size", 50*1024*1024, "download size for testing proxies")
	uploadSize           = flag.Int("upload-size", 20*1024*1024, "upload size for testing proxies (full mode only)")
	timeout              = flag.Duration("timeout", time.Second*5, "timeout for testing proxies")
	concurrent           = flag.Int("concurrent", 4, "download concurrent size")
	outputPath           = flag.String("output", "", "output config file path")
	gistToken            = flag.String("gist-token", "", "github gist token for updating output")
	gistAddress          = flag.String("gist-address", "", "github gist address or id for updating output (filename uses output basename)")
	repoToken            = flag.String("repo-token", "", "github token for updating repository file")
	repoAddress          = flag.String("repo-address", "", "github repository address or owner/repo for updating output")
	repoFilePath         = flag.String("repo-file-path", "", "repository file path for uploading output (default: output basename)")
	repoBranch           = flag.String("repo-branch", "", "repository branch for uploading output (default: repository default branch)")
	maxLatency           = flag.Duration("max-latency", time.Second, "filter latency greater than this value")
	maxPacketLoss        = flag.Float64("max-packet-loss", 100, "filter packet loss greater than this value(unit: %)")
	minDownloadSpeed     = flag.Float64("min-download-speed", 5, "filter download speed less than this value(unit: MB/s)")
	minUploadSpeed       = flag.Float64("min-upload-speed", 2, "filter upload speed less than this value(unit: MB/s, full mode only)")
	earlyStop            = flag.Int("early-stop", 0, "stop testing after this many results pass filters (0 disables)")
	renameNodes          = flag.Bool("rename", true, "rename nodes with IP location and speed")
	renameTemplate       = flag.String("rename-template", "", "name template for renaming (Go text/template). Placeholders: {{.Flag}}, {{.CountryCode}}, {{.Index}}, {{.Direction}}, {{.Speed}}, {{.SpeedUnit}}, {{.LatencyMs}}, {{.DownloadSpeedMBps}}, {{.UploadSpeedMBps}}. Empty = default format")
	fastMode             = flag.Bool("fast", false, "fast mode (alias for --speed-mode fast)")
	metricsFlag          = flag.String("metrics", "", "test metrics: latency,download,upload,antigravity or all (default follows --speed-mode)")
	durationFlag         = flag.Duration("duration", 0, "keep testing for this long, e.g. 10m (0 = one pass per node)")
	roundsFlag           = flag.Int("rounds", 1, "number of test rounds per node (default 1)")
	versionFlag          = flag.Bool("v", false, "show version information")
	userAgent            = flag.String("ua", "", "User-Agent for fetching config from http(s) URL (default: mihomo kernel UA, e.g. mihomo/1.10.0)")
	antigravityTokenFlag = flag.String("antigravity-token", "", "OAuth Bearer token (ya29...) for the Antigravity availability check")
	antigravityTokenFile = flag.String("antigravity-token-file", "", "OAuth token file for the Antigravity availability check; required when --metrics includes antigravity (file holds a bare Bearer token or an 'Authorization: Bearer ...' line)")
	guiFlag              = flag.Bool("gui", false, "launch desktop graphical user interface (GUI)")
	cliFlag              = flag.Bool("cli", false, "force terminal CLI interactive mode")
	portFlag             = flag.Int("port", 0, "port for GUI web server (default: random free port)")
	browserFlag          = flag.String("browser", "", "preferred browser for GUI: zen, arc, brave, chrome, edge, safari, default, or path to executable")
)

func main() {
	flag.Parse()
	mihomolog.SetLevel(mihomolog.SILENT)
	_ = speedtester.BindPhysicalInterface()

	// Handle version flag
	if *versionFlag {
		fmt.Printf("clash-speedtest version %s (commit %s)\n", version, commit)
		os.Exit(0)
	}

	explicitFilter := false
	explicitMetrics := false
	explicitDuration := false
	explicitRounds := false
	explicitGUI := false
	explicitCLI := false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "gui":
			explicitGUI = true
		case "cli":
			explicitCLI = true
		case "f":
			explicitFilter = true
		case "metrics":
			explicitMetrics = true
		case "duration":
			explicitDuration = true
		case "rounds":
			explicitRounds = true
		}
	})

	shouldRunGUI := false
	if explicitGUI && *guiFlag {
		shouldRunGUI = true
	} else if !explicitCLI && !*cliFlag && *configPathsConfig == "" && len(flag.Args()) == 0 {
		shouldRunGUI = true
	}

	if shouldRunGUI {
		runGUI(*portFlag, *userAgent, *browserFlag)
		return
	}

	interactiveMenu := false
	if *configPathsConfig == "" {
		if !profiles.IsInteractive() {
			log.Fatalln("please specify the configuration file")
		}
		profiles.EnableUTF8Console()
		app := profiles.NewApp(*userAgent)
		selection, err := app.SelectAirport()
		if err != nil {
			log.Fatalf("airport menu failed: %s", err)
		}
		if selection == nil {
			fmt.Println("已退出")
			os.Exit(0)
		}
		*configPathsConfig = selection.ConfigPath
		interactiveMenu = true
		fmt.Printf("使用机场: %s\n", selection.Airport.Name)
	}

	var err error
	requestedMode := speedtester.SpeedModeFast
	if !*fastMode {
		requestedMode, err = speedtester.ParseSpeedMode(*speedMode)
		if err != nil {
			log.Fatalf("parse speed mode failed: %s", err)
		}
	}
	selectedMetrics := speedtester.MetricsFromMode(requestedMode)
	if explicitMetrics {
		selectedMetrics, err = speedtester.ParseMetrics(*metricsFlag)
		if err != nil {
			log.Fatalf("parse metrics failed: %s", err)
		}
	}
	selectedDuration := *durationFlag
	selectedRounds := *roundsFlag

	loader, err := speedtester.New(&speedtester.Config{
		ConfigPaths: *configPathsConfig,
		FilterRegex: *filterRegexConfig,
		BlockRegex:  *blockKeywords,
		ServerURL:   *serverURL,
		UserAgent:   *userAgent,
		Mode:        speedtester.SpeedModeFast,
	})
	if err != nil {
		log.Fatalf("create speed tester failed: %s", err)
	}

	allProxies, err := loader.LoadProxies()
	if err != nil {
		log.Fatalf("load proxies failed: %s", err)
	}
	if len(allProxies) == 0 {
		log.Fatalln("没有读取到节点。可在菜单里选择更新订阅后再试。")
	}

	if interactiveMenu && !explicitFilter && profiles.IsInteractive() {
		names := make([]string, 0, len(allProxies))
		for name := range allProxies {
			names = append(names, name)
		}
		app := profiles.NewApp(*userAgent)
		picked, allCountries, countryErr := app.SelectCountries(names)
		if countryErr != nil {
			log.Fatalf("country menu failed: %s", countryErr)
		}
		if !allCountries {
			allProxies = keepProxies(allProxies, picked)
		}
		if len(allProxies) == 0 {
			log.Fatalln("没有符合筛选条件的节点")
		}
	}

	if interactiveMenu && profiles.IsInteractive() {
		app := profiles.NewApp(*userAgent)
		if !explicitMetrics {
			metrics, metricsErr := app.SelectMetrics()
			if metricsErr != nil {
				log.Fatalf("metrics menu failed: %s", metricsErr)
			}
			selectedMetrics = metrics
		}
		if !explicitDuration && !explicitRounds {
			duration, rounds, planErr := app.SelectPlan(selectedMetrics)
			if planErr != nil {
				log.Fatalf("plan menu failed: %s", planErr)
			}
			selectedDuration = duration
			selectedRounds = rounds
		}
		fmt.Printf("测试项目: %s\n", formatSelectedMetrics(selectedMetrics))
		if selectedDuration > 0 {
			fmt.Printf("测试时长: %s（持续监控，边测边刷新）\n", selectedDuration)
		} else if selectedRounds > 1 {
			fmt.Printf("测试轮数: %d 轮（边测边刷新）\n", selectedRounds)
		} else {
			fmt.Println("测试轮数: 1 轮（每个节点测一次）")
		}
	}

	antigravityToken := ""
	if selectedMetrics.Antigravity {
		isInteractive := profiles.IsInteractive() || output.IsTerminalFile(os.Stdin)
		token, tokenErr := speedtester.ResolveAntigravityToken(*antigravityTokenFlag, *antigravityTokenFile, isInteractive, os.Stdin, os.Stdout)
		if tokenErr != nil {
			log.Fatalf("获取 Antigravity 凭据失败: %v", tokenErr)
		}
		antigravityToken = token
	}

	speedTester, err := speedtester.New(&speedtester.Config{
		ConfigPaths:      *configPathsConfig,
		FilterRegex:      *filterRegexConfig,
		BlockRegex:       *blockKeywords,
		ServerURL:        *serverURL,
		DownloadSize:     *downloadSize,
		UploadSize:       *uploadSize,
		Timeout:          *timeout,
		Concurrent:       *concurrent,
		MaxPacketLoss:    *maxPacketLoss,
		MaxLatency:       *maxLatency,
		MinDownloadSpeed: *minDownloadSpeed * 1024 * 1024,
		MinUploadSpeed:   *minUploadSpeed * 1024 * 1024,
		Mode:             selectedMetrics.ToSpeedMode(),
		Metrics:          selectedMetrics,
		Duration:         selectedDuration,
		Rounds:           selectedRounds,
		OutputPath:       *outputPath,
		UserAgent:        *userAgent,
		AntigravityToken: antigravityToken,
	})
	if err != nil {
		log.Fatalf("create speed tester failed: %s", err)
	}
	effectiveMode := speedTester.Mode()
	resultFilter := newResultFilter(effectiveMode, speedTester.Metrics())
	stopper, err := newEarlyStopper(*earlyStop, resultFilter)
	if err != nil {
		log.Fatalf("create early stopper failed: %s", err)
	}

	outputMode := output.DetermineOutputMode(output.IsTerminalFile)

	var tsvWriter *output.TSVWriter
	if outputMode == output.OutputModeTSV {
		var err error
		tsvWriter, err = output.NewTSVWriterWithMetrics(os.Stdout, effectiveMode, selectedMetrics)
		if err != nil {
			log.Fatalf("create TSV writer failed: %s", err)
		}
	}

	latestResults := make(map[string]*speedtester.Result, len(allProxies))

	if outputMode == output.OutputModeInteractive {
		collectResults := *outputPath != ""
		resultChannel := make(chan *speedtester.Result, 256)
		resultsDone := make(chan struct{})
		saveResult := make(chan error, 1)

		go func() {
			speedTester.TestProxiesUntil(allProxies, func(result *speedtester.Result) bool {
				if collectResults {
					latestResults[result.ProxyName] = result
				}
				resultChannel <- result
				return stopper.ShouldContinue(result)
			})
			close(resultChannel)
			close(resultsDone)
		}()

		if collectResults {
			go func() {
				<-resultsDone
				results := make([]*speedtester.Result, 0, len(latestResults))
				for _, result := range latestResults {
					results = append(results, result)
				}
				results = output.SortResultsWithMetrics(results, effectiveMode, selectedMetrics)
				saveResult <- saveConfig(results, resultFilter)
			}()
		}

		p := tea.NewProgram(
			tui.NewTUIModel(effectiveMode, len(allProxies), resultChannel).WithMetrics(selectedMetrics).WithDuration(selectedDuration).WithRounds(selectedRounds),
			tea.WithAltScreen(),
			tea.WithMouseAllMotion(),
		)
		if _, err := p.Run(); err != nil {
			log.Fatalf("TUI failed: %s", err)
		}

		if !collectResults {
			return
		}

		err = <-saveResult
		if err != nil {
			log.Fatalf("save config file failed: %s", err)
		}
		fmt.Printf("\nsave config file to: %s\n", *outputPath)
		return
	}

	sampleIndex := 0
	speedTester.TestProxiesUntil(allProxies, func(result *speedtester.Result) bool {
		latestResults[result.ProxyName] = result
		if tsvWriter != nil {
			if err := tsvWriter.WriteRow(result, sampleIndex); err != nil {
				log.Printf("write TSV row failed: %s", err)
			}
			sampleIndex++
		}
		return stopper.ShouldContinue(result)
	})

	results := make([]*speedtester.Result, 0, len(latestResults))
	for _, result := range latestResults {
		results = append(results, result)
	}
	results = output.SortResultsWithMetrics(results, effectiveMode, selectedMetrics)

	if *outputPath != "" {
		err = saveConfig(results, resultFilter)
		if err != nil {
			log.Fatalf("save config file failed: %s", err)
		}
		fmt.Printf("\nsave config file to: %s\n", *outputPath)
	}
}

func saveConfig(results []*speedtester.Result, filter resultFilter) error {
	proxies := make([]map[string]any, 0)
	nameCount := make(map[string]int) // Track name usage to avoid duplicates

	for _, result := range results {
		if !filter.Match(result) {
			continue
		}

		proxyConfig := result.ProxyConfig
		if proxyConfig["name"] == nil || proxyConfig["server"] == nil {
			continue
		}
		if *renameNodes {
			location, err := ip.GetIPLocation(proxyConfig["server"].(string))
			if err != nil || location.CountryCode == "" {
				proxies = append(proxies, proxyConfig)
				continue
			}
			name, err := ip.GenerateNodeNameFromTemplate(*renameTemplate, location.CountryCode, result.Latency, result.DownloadSpeed, result.UploadSpeed, nameCount)
			if err != nil {
				log.Printf("rename template parse error: %s, use default name", err)
				name = ip.GenerateNodeName(location.CountryCode, result.Latency, result.DownloadSpeed, result.UploadSpeed, nameCount)
			}
			proxyConfig["name"] = name
		}
		proxies = append(proxies, proxyConfig)
	}

	config := &speedtester.RawConfig{
		Proxies: proxies,
	}
	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	if err := os.WriteFile(*outputPath, yamlData, 0o644); err != nil {
		return err
	}
	outputFilename := filepath.Base(filepath.Clean(*outputPath))

	if *gistToken != "" && *gistAddress != "" {
		uploader := gist.NewUploader(nil)
		if err := uploader.UpdateFile(*gistToken, *gistAddress, outputFilename, yamlData); err != nil {
			log.Printf("update gist failed: %s", err)
		}
	}

	if *repoToken != "" && *repoAddress != "" {
		uploader := gist.NewUploader(nil)
		repositoryFilePath := strings.TrimSpace(*repoFilePath)
		if repositoryFilePath == "" {
			repositoryFilePath = outputFilename
		}
		if err := uploader.UpdateRepoFile(*repoToken, *repoAddress, repositoryFilePath, *repoBranch, yamlData); err != nil {
			log.Printf("update repo file failed: %s", err)
		}
	}

	return nil
}

func formatSelectedMetrics(metrics speedtester.MetricSet) string {
	parts := make([]string, 0, 4)
	if metrics.Latency {
		parts = append(parts, "真实延迟")
	}
	if metrics.Download {
		parts = append(parts, "下载速度")
	}
	if metrics.Upload {
		parts = append(parts, "上传速度")
	}
	if metrics.Antigravity {
		parts = append(parts, "Antigravity 地区")
	}
	if len(parts) == 0 {
		return "未选择"
	}
	return strings.Join(parts, " + ")
}

func keepProxies(proxies map[string]*speedtester.CProxy, names []string) map[string]*speedtester.CProxy {
	allow := make(map[string]struct{}, len(names))
	for _, name := range names {
		allow[name] = struct{}{}
	}
	filtered := make(map[string]*speedtester.CProxy, len(allow))
	for name, proxy := range proxies {
		if _, ok := allow[name]; ok {
			filtered[name] = proxy
		}
	}
	return filtered
}

func runGUI(port int, userAgent string, browser string) {
	profiles.EnableUTF8Console()
	server, err := gui.NewServer(gui.ServerConfig{
		Port:         port,
		ProfilePaths: profiles.DefaultPaths(),
		UserAgent:    userAgent,
	})
	if err != nil {
		log.Fatalf("启动桌面 GUI 服务失败: %s", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("监听桌面服务端口失败: %s", err)
	}

	url := server.URL()
	fmt.Printf("\n==================================================\n")
	fmt.Printf(" Clash SpeedTest 桌面控制台已就绪\n")
	fmt.Printf(" 本地控制台: %s\n", url)
	fmt.Printf(" (若浏览器未自动弹出，请直接在浏览器中打开上方链接)\n")
	fmt.Printf(" (在终端按 Ctrl+C 可停止并退出服务)\n")
	fmt.Printf("==================================================\n")
	fmt.Println("正在打开桌面窗口...")

	cmd, err := gui.LaunchApp(url, browser)
	if err != nil {
		fmt.Printf("唤起桌面窗口提示: %v，已降级至系统浏览器。\n", err)
		_ = speedtester.OpenBrowser(url)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	windowClosed := make(chan struct{})
	if cmd != nil && cmd.Process != nil {
		startTime := time.Now()
		go func() {
			state, _ := cmd.Process.Wait()
			// Modern browsers (Chrome, Edge, Zen) fork or delegate to existing running instances
			// within milliseconds. Only treat as "window closed" if the process ran for more than
			// 2 seconds and exited cleanly.
			if time.Since(startTime) > 2*time.Second && (state == nil || state.Success()) {
				close(windowClosed)
			}
		}()
	}

	select {
	case <-sigChan:
		fmt.Println("\n收到退出信号，正在关闭服务...")
	case <-windowClosed:
		fmt.Println("\n桌面应用窗口已关闭，退出程序...")
	case <-server.ShutdownChan():
		fmt.Println("\n桌面应用已退出...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Stop(ctx)
}

