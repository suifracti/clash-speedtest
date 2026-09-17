package profiles

import (
	"sort"
	"strings"
	"unicode"
)

const OtherCountryCode = "OTHER"
const otherCountryCode = OtherCountryCode

type countryDef struct {
	Code  string
	Label string
	Flag  string
	Keys  []string
}

type CountryGroup struct {
	Code  string   `json:"code"`
	Label string   `json:"label"`
	Flag  string   `json:"flag"`
	Names []string `json:"names"`
}

var knownCountries = []countryDef{
	{Code: "HK", Label: "香港", Flag: "🇭🇰", Keys: []string{"香港", "广港", "滬港", "沪港", "深港", "hong kong", "hongkong", "hk"}},
	{Code: "MO", Label: "澳门", Flag: "🇲🇴", Keys: []string{"澳门", "澳門", "macao", "macau", "mo"}},
	{Code: "TW", Label: "台湾", Flag: "🇹🇼", Keys: []string{"台湾", "台灣", "台北", "taiwan", "tw"}},
	{Code: "JP", Label: "日本", Flag: "🇯🇵", Keys: []string{"日本", "东京", "大阪", "名古屋", "japan", "tokyo", "osaka", "jp"}},
	{Code: "SG", Label: "新加坡", Flag: "🇸🇬", Keys: []string{"新加坡", "狮城", "singapore", "sg"}},
	{Code: "US", Label: "美国", Flag: "🇺🇸", Keys: []string{"美国", "美國", "洛杉矶", "圣何塞", "硅谷", "西雅图", "芝加哥", "纽约", "达拉斯", "united states", "america", "usa", "us"}},
	{Code: "KR", Label: "韩国", Flag: "🇰🇷", Keys: []string{"韩国", "韓國", "首尔", "首爾", "korea", "seoul", "kr"}},
	{Code: "UK", Label: "英国", Flag: "🇬🇧", Keys: []string{"英国", "英國", "伦敦", "london", "england", "uk", "gb"}},
	{Code: "DE", Label: "德国", Flag: "🇩🇪", Keys: []string{"德国", "德國", "法兰克福", "germany", "frankfurt", "de"}},
	{Code: "FR", Label: "法国", Flag: "🇫🇷", Keys: []string{"法国", "法國", "巴黎", "france", "paris", "fr"}},
	{Code: "NL", Label: "荷兰", Flag: "🇳🇱", Keys: []string{"荷兰", "荷蘭", "阿姆斯特丹", "netherlands", "amsterdam", "nl"}},
	{Code: "AU", Label: "澳大利亚", Flag: "🇦🇺", Keys: []string{"澳大利亚", "澳洲", "悉尼", "australia", "sydney", "au"}},
	{Code: "CA", Label: "加拿大", Flag: "🇨🇦", Keys: []string{"加拿大", "canada", "toronto", "ca"}},
	{Code: "RU", Label: "俄罗斯", Flag: "🇷🇺", Keys: []string{"俄罗斯", "俄羅斯", "俄国", "russia", "moscow", "ru"}},
	{Code: "TH", Label: "泰国", Flag: "🇹🇭", Keys: []string{"泰国", "泰國", "曼谷", "thailand", "bangkok", "th"}},
	{Code: "MY", Label: "马来西亚", Flag: "🇲🇾", Keys: []string{"马来西亚", "馬來西亞", "malaysia", "my"}},
	{Code: "VN", Label: "越南", Flag: "🇻🇳", Keys: []string{"越南", "vietnam", "vn"}},
	{Code: "PH", Label: "菲律宾", Flag: "🇵🇭", Keys: []string{"菲律宾", "菲律賓", "philippines", "manila", "ph"}},
	{Code: "ID", Label: "印尼", Flag: "🇮🇩", Keys: []string{"印度尼西亚", "印尼", "indonesia"}},
	{Code: "IN", Label: "印度", Flag: "🇮🇳", Keys: []string{"印度", "india", "mumbai"}},
	{Code: "TR", Label: "土耳其", Flag: "🇹🇷", Keys: []string{"土耳其", "turkey", "istanbul", "tr"}},
	{Code: "AE", Label: "阿联酋", Flag: "🇦🇪", Keys: []string{"阿联酋", "阿聯酋", "迪拜", "dubai", "ae"}},
	{Code: "CN", Label: "中国", Flag: "🇨🇳", Keys: []string{"中国", "中國", "大陆", "大陸", "回国", "china", "cn"}},
	{Code: "AR", Label: "阿根廷", Flag: "🇦🇷", Keys: []string{"阿根廷", "argentina"}},
	{Code: "BR", Label: "巴西", Flag: "🇧🇷", Keys: []string{"巴西", "brazil"}},
	{Code: "IT", Label: "意大利", Flag: "🇮🇹", Keys: []string{"意大利", "italy", "milan", "it"}},
	{Code: "ES", Label: "西班牙", Flag: "🇪🇸", Keys: []string{"西班牙", "spain", "madrid", "es"}},
	{Code: "SE", Label: "瑞典", Flag: "🇸🇪", Keys: []string{"瑞典", "sweden", "se"}},
	{Code: "CH", Label: "瑞士", Flag: "🇨🇭", Keys: []string{"瑞士", "switzerland", "ch"}},
	{Code: "PL", Label: "波兰", Flag: "🇵🇱", Keys: []string{"波兰", "波蘭", "poland", "pl"}},
}

type countryKey struct {
	key  string
	code string
}

var (
	countryByCode  = map[string]countryDef{}
	countryByLabel = map[string]string{}
	countryKeys    []countryKey
)

func init() {
	for _, def := range knownCountries {
		countryByCode[def.Code] = def
		countryByLabel[strings.ToLower(def.Label)] = def.Code
		countryByLabel[strings.ToLower(def.Code)] = def.Code
		if def.Code == "UK" {
			countryByLabel["gb"] = "UK"
		}
		for _, key := range def.Keys {
			countryKeys = append(countryKeys, countryKey{key: key, code: def.Code})
		}
	}
	sort.SliceStable(countryKeys, func(i, j int) bool {
		ri, rj := []rune(countryKeys[i].key), []rune(countryKeys[j].key)
		if len(ri) != len(rj) {
			return len(ri) > len(rj)
		}
		return countryKeys[i].key < countryKeys[j].key
	})
}

func DetectCountry(name string) (code string) {
	lower := strings.ToLower(name)
	tokens := nameTokens(name)

	// Explicit text region keywords take precedence over flag emojis (e.g. 🇨🇳台湾 or 🇨🇳香港)
	for _, rule := range countryKeys {
		if matchCountryKey(lower, tokens, rule.key) {
			return rule.code
		}
	}

	if flagCode := countryFromFlag(name); flagCode != "" {
		return normalizeCountryCode(flagCode)
	}
	return OtherCountryCode
}

func GroupByCountry(names []string) []CountryGroup {
	grouped := make(map[string]*CountryGroup)
	order := make([]string, 0)
	for _, name := range names {
		code := DetectCountry(name)
		if code == OtherCountryCode || code == "" {
			continue // "其他不是地区": omit non-region groups
		}
		group, ok := grouped[code]
		if !ok {
			def, known := countryByCode[code]
			group = &CountryGroup{Code: code}
			if known {
				group.Label = def.Label
				group.Flag = def.Flag
			} else {
				group.Label = code
				group.Flag = flagFromCode(code)
			}
			grouped[code] = group
			order = append(order, code)
		}
		group.Names = append(group.Names, name)
	}

	groups := make([]CountryGroup, 0, len(order))
	for _, code := range order {
		groups = append(groups, *grouped[code])
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if len(groups[i].Names) != len(groups[j].Names) {
			return len(groups[i].Names) > len(groups[j].Names)
		}
		return groups[i].Label < groups[j].Label
	})
	return groups
}

func LookupCountryCode(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}
	if code, ok := countryByLabel[strings.ToLower(trimmed)]; ok {
		return code, true
	}
	upper := strings.ToUpper(trimmed)
	if upper == "GB" {
		return "UK", true
	}
	if _, ok := countryByCode[upper]; ok {
		return upper, true
	}
	return "", false
}

func matchCountryKey(lowerName string, tokens map[string]struct{}, key string) bool {
	if key == "" {
		return false
	}
	if isShortCodeKey(key) {
		_, ok := tokens[key]
		return ok
	}
	return strings.Contains(lowerName, key)
}

func isShortCodeKey(key string) bool {
	if len(key) != 2 {
		return false
	}
	for _, r := range key {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func nameTokens(name string) map[string]struct{} {
	tokens := make(map[string]struct{})
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens[strings.ToLower(b.String())] = struct{}{}
		b.Reset()
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func countryFromFlag(name string) string {
	runes := []rune(name)
	for i := 0; i < len(runes)-1; i++ {
		if isRegionalIndicator(runes[i]) && isRegionalIndicator(runes[i+1]) {
			c0 := runes[i] - 0x1F1E6 + 'A'
			c1 := runes[i+1] - 0x1F1E6 + 'A'
			return string([]rune{c0, c1})
		}
	}
	return ""
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

func normalizeCountryCode(code string) string {
	code = strings.ToUpper(code)
	if code == "GB" {
		return "UK"
	}
	return code
}

func FlagFromCode(code string) string {
	return flagFromCode(code)
}

func LabelFromCode(code string) string {
	if def, ok := countryByCode[strings.ToUpper(code)]; ok {
		return def.Label
	}
	if code == otherCountryCode || code == "" {
		return "其他"
	}
	return code
}

func flagFromCode(code string) string {
	if def, ok := countryByCode[code]; ok {
		return def.Flag
	}
	code = normalizeCountryCode(code)
	if len(code) != 2 {
		return "🏳️"
	}
	r := []rune(code)
	return string([]rune{0x1F1E6 + (r[0] - 'A'), 0x1F1E6 + (r[1] - 'A')})
}
