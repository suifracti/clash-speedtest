package ip

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type IPLocation struct {
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
}

func GetIPLocation(ip string) (*IPLocation, error) {
	cleanIP := strings.TrimSpace(ip)
	if cleanIP == "" {
		return nil, fmt.Errorf("ip is empty")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://ip-api.com/json/%s?fields=country,countryCode", cleanIP))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get location for IP %s", cleanIP)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if err != nil {
		return nil, err
	}

	var location IPLocation
	if err := json.Unmarshal(body, &location); err != nil {
		return nil, err
	}
	return &location, nil
}
