<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import * as api from '../../api/bridge'

const store = useWorkbenchStore()

const tokenInput = ref('')
const isSavingToken = ref(false)
const tokenSuccess = ref(false)
const tokenError = ref('')

onMounted(() => {
  tokenInput.value = store.tokenStatus.preview || ''
})

function closeModal() {
  store.isSettingsModalOpen = false
  tokenError.value = ''
  tokenSuccess.value = false
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
    class="fixed inset-0 bg-black/60 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none"
  >
    <div class="bg-card border border-border rounded-xl shadow-2xl w-full max-w-lg overflow-hidden text-xs">
      <!-- Modal Header -->
      <div class="p-4 border-b border-border flex items-center justify-between">
        <h2 class="text-sm font-bold text-content-main flex items-center gap-2">
          <span>⚙️</span> 运行偏好与凭据配置
        </h2>
        <button @click="closeModal" class="text-content-muted hover:text-content-main text-lg font-mono">
          ✕
        </button>
      </div>

      <!-- Modal Body -->
      <div class="p-5 flex flex-col gap-5">
        <!-- Antigravity Credentials Section -->
        <div class="flex flex-col gap-2.5">
          <div class="flex items-center justify-between">
            <span class="font-semibold text-content-main">Google Antigravity 凭据</span>
            <span
              v-if="store.tokenStatus.has_token"
              class="text-[11px] text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/20"
            >
              已绑定 ({{ store.tokenStatus.source }})
            </span>
          </div>

          <p class="text-[11px] text-content-secondary leading-relaxed">
            用于评估 Google 是否对出境节点执行区域封锁 (FAILED_PRECONDITION) 及测量真实的 API TTFB 耗时。
          </p>

          <div class="flex flex-col gap-2">
            <div class="flex items-center gap-2">
              <input
                v-model="tokenInput"
                type="password"
                placeholder="Bearer ya29... 或直接粘贴完整凭据"
                class="flex-1 bg-card-subtle text-content-main border border-border rounded px-3 py-1.5 focus:outline-none focus:border-brand font-mono text-[11px]"
              />
              <button
                @click="handleSaveToken"
                :disabled="isSavingToken"
                class="px-3 py-1.5 rounded bg-blue-600 hover:bg-blue-500 text-white font-medium disabled:opacity-50"
              >
                {{ isSavingToken ? '保存中' : '更新' }}
              </button>
            </div>

            <div v-if="tokenError" class="text-red-400 text-[11px]">
              {{ tokenError }}
            </div>
            <div v-if="tokenSuccess" class="text-emerald-400 text-[11px]">
              凭据已成功保存生效！
            </div>
          </div>

          <div class="flex items-center justify-between pt-2 border-t border-border">
            <span class="text-content-muted text-[11px]">或者通过浏览器授权:</span>
            <button
              @click="handleOAuthLogin"
              class="px-3 py-1 rounded bg-card-subtle hover:bg-card border border-border text-content-main font-medium flex items-center gap-1.5"
            >
              <span>🔑</span> 启动 Google OAuth 授权
            </button>
          </div>
        </div>

        <!-- System & UI Section -->
        <div class="flex flex-col gap-2 pt-4 border-t border-border">
          <span class="font-semibold text-content-main">视觉偏好</span>
          <div class="flex items-center justify-between text-content-secondary">
            <span>主题配色</span>
            <span class="font-mono text-content-muted">深色工业模式 (Dark)</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
