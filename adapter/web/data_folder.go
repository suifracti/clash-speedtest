package web

import (
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

func (s *Server) handleOpenDataFolder(w http.ResponseWriter, r *http.Request) {
	setup, err := s.app.GetProfileSetup()
	if err != nil {
		writeError(w, 500, "无法读取数据位置")
		return
	}
	path := setup.DataRoot
	if r.URL.Query().Get("kind") == "monitor" {
		path = setup.HistoryDir
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		writeError(w, 400, "数据目录不存在")
		return
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("explorer.exe", path)
	case "darwin":
		command = exec.Command("open", path)
	default:
		command = exec.Command("xdg-open", path)
	}
	if err := command.Start(); err != nil {
		writeError(w, 500, "无法打开文件夹，请复制路径手动打开")
		return
	}
	go func() { _ = command.Wait() }()
	writeJSON(w, 200, map[string]bool{"success": true})
}
