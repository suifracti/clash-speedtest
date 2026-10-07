package application

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type PeriodicCycleRecord struct {
	Kind  string        `json:"kind"`
	Cycle PeriodicCycle `json:"cycle"`
}

func (p *periodicSampling) archivePeriodicCycleLocked(kind string, cycle PeriodicCycle) {
	// Offline fixtures have no storage. Production summaries live beside the
	// private configuration and do not change or replace SQLite measurements.
	if p.app == nil || p.app.appPaths.DataRoot == "" {
		return
	}
	p.recent = append(p.recent, PeriodicCycleRecord{kind, cycle})
	if len(p.recent) > 20 {
		p.recent = p.recent[len(p.recent)-20:]
	}
	data, err := json.MarshalIndent(p.recent, "", "  ")
	if err == nil {
		path := filepath.Join(p.app.appPaths.DataRoot, "periodic_sampling_cycles.json")
		err = os.WriteFile(path+".tmp", data, 0600)
		if err == nil {
			err = os.Rename(path+".tmp", path)
		}
	}
	if err != nil {
		p.lastError = "巡检轮次摘要保存失败，原始测量历史保留：" + err.Error()
	}
}
func (p *periodicSampling) loadPeriodicCycles() {
	data, err := os.ReadFile(filepath.Join(p.app.appPaths.DataRoot, "periodic_sampling_cycles.json"))
	if os.IsNotExist(err) {
		return
	}
	if err == nil {
		err = json.Unmarshal(data, &p.recent)
	}
	if err != nil {
		p.lastError = "巡检轮次摘要读取失败：" + err.Error()
		p.recent = nil
	}
	if len(p.recent) > 20 {
		p.recent = p.recent[len(p.recent)-20:]
	}
}
