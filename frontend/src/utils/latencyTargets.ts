import type { WorkbenchLatencyTest } from '../types'

export const latencyTargets = [
  { value: 'cloudflare', label: 'Cloudflare', url: 'https://speed.cloudflare.com/__down?bytes=1' },
  { value: 'google', label: 'Google', url: 'https://www.gstatic.com/generate_204' },
  { value: 'github', label: 'GitHub', url: 'https://api.github.com/zen' },
  { value: 'apple', label: 'Apple', url: 'https://captive.apple.com/hotspot-detect.html' },
  { value: 'microsoft', label: 'Microsoft', url: 'http://www.msftconnecttest.com/connecttest.txt' },
  { value: 'firefox', label: 'Firefox', url: 'https://detectportal.firefox.com/success.txt' },
]

// Keep long-term stability comparable: only the same baseline site enters its chart.
export function baselineLatencyTest(test: WorkbenchLatencyTest): WorkbenchLatencyTest {
  if (test.target !== 'multi://latency-v1') return test
  const samples = test.samples.filter(sample => sample.target === latencyTargets[0]!.url)
  const successes = samples.filter(sample => sample.success)
  return { ...test, target: latencyTargets[0]!.url, samples, total_samples: samples.length,
    success_samples: successes.length, failure_samples: samples.length - successes.length,
    latency_ms: successes.length ? Math.round(successes.reduce((sum, sample) => sum + sample.latency_ms, 0) / successes.length) : 0,
    status: !successes.length ? 'failed' : successes.length === samples.length ? 'completed' : 'partial_failed',
    error_message: samples.find(sample => !sample.success)?.error || '',
  }
}

export function latencySiteResults(tests: WorkbenchLatencyTest[]) {
  return latencyTargets.map(target => {
    const test = tests.find(test => test.target === 'multi://latency-v1' || (test.target || latencyTargets[0]!.url) === target.url)
    const samples = test?.samples.filter(sample => (sample.target || test.target || latencyTargets[0]!.url) === target.url) || []
    const successes = samples.filter(sample => sample.success)
    return { ...target, count: samples.length, successes: successes.length,
      latency: successes.length ? Math.round(successes.reduce((sum, sample) => sum + sample.latency_ms, 0) / successes.length) : null,
      error: [...new Set(samples.filter(sample => !sample.success).map(sample => sample.error || '请求失败'))].join('；'),
      time: test?.finished_at || '',
    }
  })
}

// Only compare coverage within the latest suite; old single-site runs do not
// imply that the other five sites failed or were tested at the same time.
export function suiteHealth(tests: WorkbenchLatencyTest[]) {
  const suite = tests.find(test => test.target === 'multi://latency-v1')
  if (!suite) return null
  const sites = latencySiteResults([suite])
  const tested = sites.filter(site => site.count > 0)
  const reachable = tested.filter(site => site.successes > 0).length
  const complete = tested.filter(site => site.successes === site.count).length
  const count = tested.reduce((sum, site) => sum + site.count, 0)
  const success = tested.reduce((sum, site) => sum + site.successes, 0)
  return { tested: tested.length, reachable, complete, count, success,
    failures: tested.filter(site => site.successes < site.count).map(site => ({ label: site.label, failed: site.count - site.successes, count: site.count })),
    rate: count ? `${(success / count * 100).toFixed(1)}%` : '—',
    label: !count ? '暂无样本' : !reachable ? '本轮各站均失败' : complete === 6 ? '本轮六站全部通过' : '本轮部分请求失败',
    time: suite.finished_at,
  }
}
