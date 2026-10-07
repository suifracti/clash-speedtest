package application

import (
	"regexp"
	"strings"
	"unicode"
)

// Match the existing Home view's announcement classification, so metadata
// entries do not become recurring proxy or real-model requests.
var periodicNoticePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(公告|机场公告|通知|重要提示|订阅地址(失效|过期)|使用说明|官网地址|官方网址|官方地址|防失联(页|地址|网址)?|电报群|交流群|官方(群|频道)|telegram\s*(group|channel)|tg\s*(群|频道))`),
	regexp.MustCompile(`(?i)(电报群|交流群|防失联|官网|官方地址|备用网址|使用文档|客户端).*(https?://|t\.me/)|^https?://(t\.me|telegram\.me|github\.com)/`),
	regexp.MustCompile(`^(剩余流量|套餐到期|到期时间|订阅到期|套餐有效期|有效期[至:：]|流量重置(时间|日期|[：:])|订阅更新(时间|日期|[：:]))`),
	regexp.MustCompile(`^(如果只(显示|看到)(此|一个)?节点|是客户端太旧|更新(一下|你的)?客户端|请到(官网|网站).*(教程|使用说明)|请重新(复制)?导入)`),
}
var periodicBracketNotice = regexp.MustCompile(`(?i)^\[[^\]\r\n]{1,30}\][✨\s]*(永久|官网|网址|备用)[:：]{2,}[^|\s]+\.[a-z]{2,}($|\s)`)
var periodicInstructionNotice = regexp.MustCompile(`每次使用前.*更新订阅|(请|使用前|务必).*(重新导入|更新订阅|阅读.*(说明|文档)|下载.*客户端)|(用不了|无法使用).*(客户端|重新导入)|(不够|不会使用).*(官网|使用文档|客户端)`)
var periodicRegion = regexp.MustCompile(`(?i)^[A-Z]{2}$`)

func periodicNotice(n MonitorNodeOptionDTO) bool {
	name := strings.TrimLeftFunc(strings.TrimSpace(n.DisplayName), func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r) })
	for _, pattern := range periodicNoticePatterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	if periodicBracketNotice.MatchString(strings.TrimSpace(n.DisplayName)) {
		return true
	}
	code := strings.ToUpper(n.CountryCode)
	if periodicRegion.MatchString(code) && code != "XX" && code != "ZZ" {
		return false
	}
	return periodicInstructionNotice.MatchString(name)
}
