export function serviceOutcomeLabel(value?: string): string {
  return ({ matched: '检测通过', profiled: '画像已获取', reachable: '网页可达', challenge: '需要浏览器验证', unlocked: '地区 / 片目判据通过', originals_only: '仅宽松片目通过', region_limited: '仅海外 / 非目标地区档位', service_rejected: '服务拒绝请求', credentials_required: '请先绑定 Google 凭据', auth_failed: '凭据失效，请重新绑定', permission_denied: '账号或项目权限不足', setup_required: '账号配置尚未完成', region_blocked: '服务明确拒绝当前地区', unknown: '暂时无法确认可用性', http_rejected: '服务拒绝请求', rate_limited: '额度或请求频率受限', redirect: '重定向未跟随', timed_out: '检测超时', cancelled: '已取消', transport_error: '连接或响应读取失败', criteria_mismatch: '响应未符合检测规则' } as Record<string,string>)[value || ''] || '尚无检测结果'
}

export function serviceDetailLabel(key: string): string {
  return ({
    ip: '出口 IP', country: '国家', country_code: '国家代码', location: '位置', asn: 'ASN',
    organization: '网络组织', ip_type: '网络类型', origin_type: 'IP 来源', risk_score: 'Ping0 风险分',
    fraud_score: 'IPPure 欺诈分', cloudflare_colo: 'Cloudflare 机房', tls: 'TLS', http_protocol: 'HTTP',
    redirect_to: '跳转目标', final_url: '最终地址', service_region: '服务地区', global_title: '宽松片目', licensed_title: '版权片目',
    exit_observation: '出口观察说明', exit_observation_state: '出口观察状态', dns_status: 'DNS 状态', answer_count: 'DNS 答案数',
    ip_family: 'IP 类型', exit_scope: '出口适用范围', proxy_connect_ms: '代理建连累计（毫秒）',
    tls_handshake_ms: 'TLS 握手累计（毫秒）', first_response_byte_ms: '请求至首字节累计（毫秒）',
    dns_ms: '可见 DNS 耗时（毫秒）', dns_timing_note: 'DNS 计时说明', phase_timing_note: '阶段计时说明',
    antigravity_progress: 'Antigravity 验证到哪一步', checked_model: '本次测试模型',
  } as Record<string, string>)[key] || key
}
