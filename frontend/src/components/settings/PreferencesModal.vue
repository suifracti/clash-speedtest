<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import * as api from '../../api/bridge'

const store = useWorkbenchStore()

const tokenInput = ref('')
const isSavingToken = ref(false)
const tokenSuccess = ref(false)
const tokenError = ref('')
const isDark = ref(false)

onMounted(() => {
  tokenInput.value = ''
  isDark.value = document.documentElement.classList.contains('dark') || document.body.classList.contains('dark-theme')
})

function closeModal() {
  store.isSettingsModalOpen = false
  tokenError.value = ''
  tokenSuccess.value = false
}

function toggleTheme() {
  isDark.value = !isDark.value
  if (isDark.value) {
    document.documentElement.classList.add('dark', 'dark-theme')
    document.body.classList.add('dark', 'dark-theme')
    localStorage.setItem('theme', 'dark')
  } else {
    document.documentElement.classList.remove('dark', 'dark-theme')
    document.body.classList.remove('dark', 'dark-theme')
    localStorage.setItem('theme', 'light')
  }
}

async function handleSaveToken() {
  tokenError.value = ''
  tokenSuccess.value = false
  if (!tokenInput.value.trim()) {
    tokenError.value = 'Token 不能为空'
    return
  }

  isSavingToken.value = true
  try {
    await api.setToken(tokenInput.value.trim())
    await store.loadTokenStatus()
    tokenSuccess.value = true
  } catch (e: any) {
    tokenError.value = e.message || 'Token 验证失败'
  } finally {
    isSavingToken.value = false
  }
}

async function handleOAuthLogin() {
  try {
    await api.startOAuthLogin()
  } catch (e: any) {
    tokenError.value = e.message || 'OAuth 登录失败'
  }
}
</script>

<template>
  <div
    v-if="store.isSettingsModalOpen"
    class="fixed inset-0 bg-black/40 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none"
  >
    <div class="prototype-modal w-full max-w-lg overflow-hidden text-xs">
      <!-- Modal Header -->
      <div class="prototype-modal-header">
        <h2 class="prototype-modal-title">
          <svg class="w-4 h-4 text-primary" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
            <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
          </svg>
          设置
        </h2>
        <button @click="closeModal" class="prototype-close-btn font-mono" aria-label="关闭">
          ✕
        </button>
      </div>

      <!-- Modal Body -->
      <div class="prototype-modal-body flex flex-col gap-5">
        <section class="settings-data"><div><strong>数据与备份</strong><p>打开本机文件夹，导出备份或迁移监测配置。</p></div><button class="tool-button" @click="closeModal(); store.isProfileSetupOpen = true">管理数据 →</button></section>
        <!-- Antigravity Credentials Section -->
        <div class="flex flex-col gap-2.5">
          <div class="flex items-center justify-between">
            <span class="font-semibold text-content-main">Google Antigravity 凭据</span>
            <span
              v-if="store.tokenStatus.has_token"
              class="badge badge--success font-mono"
            >
              已绑定 ({{ store.tokenStatus.source }})
            </span>
            <span v-else class="badge badge--neutral">未绑定</span>
          </div>

          <p class="text-[11px] text-content-secondary leading-relaxed">
            仅用于明确选择的 Antigravity 服务检测；不影响普通延迟与下载测试。
          </p>

          <div class="flex flex-col gap-2">
            <div class="flex items-center gap-2">
              <input
                v-model="tokenInput"
                type="password"
                placeholder="Bearer ya29... 或直接粘贴完整凭据"
                class="prototype-input flex-1 font-mono text-[11px]"
              />
              <button
                @click="handleSaveToken"
                :disabled="isSavingToken"
                class="prototype-btn-primary"
              >
                {{ isSavingToken ? '保存中' : '更新' }}
              </button>
            </div>

            <div v-if="tokenError" class="text-red-500 text-[11px]">
              {{ tokenError }}
            </div>
            <div v-if="tokenSuccess" class="text-emerald-600 dark:text-emerald-400 text-[11px]">
              凭据已成功保存生效！
            </div>
          </div>

          <div class="flex items-center justify-between pt-2.5 border-t border-border">
            <span class="text-content-muted text-[11px]">或者通过浏览器授权:</span>
            <button
              @click="handleOAuthLogin"
              class="tool-button flex items-center gap-1.5"
            >
              <svg class="w-3.5 h-3.5 text-primary" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
              </svg>
              启动 Google OAuth 授权
            </button>
          </div>
        </div>

        <!-- System & UI Section -->
        <div class="flex flex-col gap-2.5 pt-4 border-t border-border">
          <span class="font-semibold text-content-main">视觉偏好</span>
          <div class="flex items-center justify-between text-content-secondary">
            <span>主题配色</span>
            <button
              type="button"
              class="tool-button flex items-center gap-1.5 font-medium"
              @click="toggleTheme"
            >
              <span v-if="isDark">🌙 深色模式</span>
              <span v-else>☀️ 浅色模式</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.prototype-modal { border-radius: 16px; }
.prototype-modal-header { background: var(--primary-subtle); padding: 22px 24px; }
.prototype-modal-body { padding: 24px; }
.prototype-modal-body > div { background: var(--card-subtle); border: 1px solid var(--border); border-radius: 10px; padding: 18px; }
.settings-data { display: flex; justify-content: space-between; align-items: center; gap: 16px; padding: 16px; border-left: 3px solid #5276aa; background: var(--card-subtle); }
.settings-data p { font-size: 11px; color: var(--text-secondary); margin-top: 5px; }
</style>
