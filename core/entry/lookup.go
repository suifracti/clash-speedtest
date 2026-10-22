package entry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Public IP source: the same ip-api.com endpoint core/ip already uses, so an
// entry address and an exit address are described by the same provider.
const (
	LookupBase = "http://ip-api.com"
	batchLimit = 100
)

var lookupFields = "status,message,country,countryCode,city,isp,as,query"

type lookupItem struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	City        string `json:"city"`
	ISP         string `json:"isp"`
	AS          string `json:"as"`
	Query       string `json:"query"`
}

func (item lookupItem) address(ip string) Address {
	asn := strings.TrimSpace(item.AS)
	if index := strings.IndexByte(asn, ' '); index > 0 {
		asn = asn[:index]
	}
	return Address{
		IP:          ip,
		CountryCode: strings.ToUpper(strings.TrimSpace(item.CountryCode)),
		Country:     strings.TrimSpace(item.Country),
		City:        strings.TrimSpace(item.City),
		ISP:         strings.TrimSpace(item.ISP),
		ASN:         strings.ToUpper(asn),
		QueriedAt:   time.Now().UTC(),
	}
}

// Lookup resolves every address the caller passes, in batches of batchLimit.
// A per-address failure is returned in failed instead of discarding the whole
// answer, so one unlucky address never blanks out a node's entry.
func Lookup(ctx context.Context, client *http.Client, ips []string) (map[string]Address, map[string]string, error) {
	addressed := make([]string, 0, len(ips))
	seen := map[string]bool{}
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		addressed = append(addressed, ip)
	}
	if len(addressed) == 0 {
		return map[string]Address{}, map[string]string{}, nil
	}
	if client == nil {
		return nil, nil, fmt.Errorf("no physical HTTP client")
	}
	addresses := make(map[string]Address, len(addressed))
	failed := map[string]string{}
	for start := 0; start < len(addressed); start += batchLimit {
		end := min(start+batchLimit, len(addressed))
		chunk := addressed[start:end]
		chunkAddresses, chunkFailed, err := lookupBatch(ctx, client, chunk)
		if err != nil {
			// The first batch failing means the source is unusable; a later
			// batch failing only costs its own addresses.
			if start == 0 {
				return nil, nil, err
			}
			for _, ip := range chunk {
				failed[ip] = err.Error()
			}
			continue
		}
		for ip, address := range chunkAddresses {
			addresses[ip] = address
		}
		for ip, reason := range chunkFailed {
			failed[ip] = reason
		}
	}
	return addresses, failed, nil
}

func lookupBatch(ctx context.Context, client *http.Client, addressed []string) (map[string]Address, map[string]string, error) {
	payload, err := json.Marshal(addressed)
	if err != nil {
		return nil, nil, err
	}
	url := LookupBase + "/batch?fields=" + lookupFields
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("IP 查询返回 HTTP %d", resp.StatusCode)
	}
	var items []lookupItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, nil, fmt.Errorf("IP 查询返回内容无法解析")
	}
	addresses := make(map[string]Address, len(items))
	failed := map[string]string{}
	for index, item := range items {
		ip := strings.TrimSpace(item.Query)
		if ip == "" && index < len(addressed) {
			ip = addressed[index]
		}
		if ip == "" {
			continue
		}
		if item.Status != "success" {
			message := strings.TrimSpace(item.Message)
			if message == "" {
				message = "查询未成功"
			}
			failed[ip] = message
			continue
		}
		addresses[ip] = item.address(ip)
	}
	return addresses, failed, nil
}

// LookupLocal asks the source what address and carrier this machine leaves from.
// The caller must pass a client bound to the physical interface.
func LookupLocal(ctx context.Context, client *http.Client) (Local, error) {
	if client == nil {
		return Local{}, fmt.Errorf("no physical HTTP client")
	}
	url := LookupBase + "/json/?fields=" + lookupFields
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Local{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Local{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return Local{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Local{}, fmt.Errorf("本机位置查询返回 HTTP %d", resp.StatusCode)
	}
	var item lookupItem
	if err := json.Unmarshal(body, &item); err != nil {
		return Local{}, fmt.Errorf("本机位置返回内容无法解析")
	}
	if item.Status != "success" {
		message := strings.TrimSpace(item.Message)
		if message == "" {
			message = "查询未成功"
		}
		return Local{}, fmt.Errorf("%s", message)
	}
	address := item.address(item.Query)
	return Local{
		CountryCode: address.CountryCode,
		Country:     address.Country,
		ISP:         address.ISP,
		IPMasked:    MaskIP(address.IP),
		QueriedAt:   address.QueriedAt,
	}, nil
}
