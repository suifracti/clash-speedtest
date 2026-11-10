package profiles

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// VergeProfile is a local source record. URLs remain inside the backend.
type VergeProfile struct {
	UID       string
	Name      string
	URL       string
	File      string
	UpdatedAt time.Time
	Usage     *SubscriptionUsage
}

func DefaultVergeProfilesFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("无法定位 Clash Verge 配置目录")
	}
	return filepath.Join(dir, "io.github.clash-verge-rev.clash-verge-rev", "profiles.yaml"), nil
}

func LoadVergeProfiles(indexPath string) ([]VergeProfile, error) {
	body, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, errors.New("无法读取 Clash Verge 订阅列表")
	}
	var index struct {
		Items []struct {
			UID     string             `yaml:"uid"`
			Type    string             `yaml:"type"`
			Name    string             `yaml:"name"`
			URL     string             `yaml:"url"`
			File    string             `yaml:"file"`
			Updated int64              `yaml:"updated"`
			Usage   *SubscriptionUsage `yaml:"extra"`
		} `yaml:"items"`
	}
	if err = yaml.Unmarshal(body, &index); err != nil {
		return nil, errors.New("Clash Verge 订阅列表格式无效")
	}
	profiles := make([]VergeProfile, 0, len(index.Items))
	for _, item := range index.Items {
		uid, source, file := strings.TrimSpace(item.UID), strings.TrimSpace(item.URL), strings.TrimSpace(item.File)
		if item.Type != "remote" || uid == "" || source == "" || file == "" {
			continue
		}
		profile := VergeProfile{UID: uid, Name: strings.TrimSpace(item.Name), URL: source, File: file, Usage: item.Usage}
		if item.Updated > 0 {
			profile.UpdatedAt = time.Unix(item.Updated, 0).UTC()
		}
		if profile.Usage != nil {
			profile.Usage.UpdatedAt = profile.UpdatedAt
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

// ReadVergeProfileCache imports node definitions, not Verge's root settings.
func ReadVergeProfileCache(indexPath string, profile VergeProfile) ([]byte, error) {
	if profile.File == "" || profile.File == "." || profile.File == ".." || strings.ContainsAny(profile.File, `/\`) {
		return nil, errors.New("Clash Verge 订阅缓存文件名无效")
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(indexPath), "profiles", profile.File))
	if err != nil {
		return nil, errors.New("无法读取 Clash Verge 订阅缓存，请先在 Clash Verge 更新订阅")
	}
	var document struct {
		Proxies yaml.Node `yaml:"proxies"`
	}
	if err = yaml.Unmarshal(body, &document); err != nil || document.Proxies.Kind != yaml.SequenceNode || len(document.Proxies.Content) == 0 {
		return nil, errors.New("Clash Verge 订阅缓存没有有效节点")
	}
	filtered := struct {
		Proxies *yaml.Node `yaml:"proxies"`
	}{Proxies: &document.Proxies}
	result, err := yaml.Marshal(filtered)
	if err != nil {
		return nil, errors.New("无法读取 Clash Verge 节点配置")
	}
	return result, nil
}
