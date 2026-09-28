<script setup lang="ts">
import { ref } from 'vue'
import * as api from '../../api/bridge'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  (e: 'authenticated'): void
}>()

const password = ref('')
const error = ref('')
const loading = ref(false)

async function submitLogin() {
  if (!password.value) {
    error.value = '请输入控制台访问密码'
    return
  }
  loading.value = true
  error.value = ''
  try {
    const res = await api.loginWithPassword(password.value)
    if (res.authenticated) {
      password.value = ''
      emit('authenticated')
    } else {
      error.value = '密码错误'
    }
  } catch (err: any) {
    error.value = err.message || '登录失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div v-if="visible" class="auth-modal-overlay">
    <div class="auth-modal-card" role="dialog" aria-modal="true" aria-labelledby="auth-title">
      <div class="auth-modal-header">
        <span class="auth-icon">🔒</span>
        <h2 id="auth-title">控制台身份验证</h2>
        <p>当前服务端启用了公网访问保护，请输入访问密码以继续。</p>
      </div>

      <form class="auth-modal-form" @submit.prevent="submitLogin">
        <label class="auth-input-label">
          <span>访问密码</span>
          <input
            v-model="password"
            type="password"
            placeholder="请输入密码…"
            autocomplete="current-password"
            autofocus
            :disabled="loading"
          >
        </label>

        <p v-if="error" class="auth-error-msg" role="alert">{{ error }}</p>

        <div class="auth-modal-actions">
          <button type="submit" class="prototype-button primary full-width" :disabled="loading">
            {{ loading ? '验证中…' : '解锁控制台' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.auth-modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(10, 15, 29, 0.85);
  backdrop-filter: blur(8px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  padding: 1.5rem;
}

.auth-modal-card {
  width: 100%;
  max-width: 420px;
  background: #1e293b;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 16px;
  padding: 2rem;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
  color: #f8fafc;
}

.auth-modal-header {
  text-align: center;
  margin-bottom: 1.5rem;
}

.auth-icon {
  font-size: 2.5rem;
  display: block;
  margin-bottom: 0.5rem;
}

.auth-modal-header h2 {
  font-size: 1.35rem;
  font-weight: 700;
  margin: 0 0 0.5rem 0;
}

.auth-modal-header p {
  font-size: 0.875rem;
  color: #94a3b8;
  margin: 0;
  line-height: 1.4;
}

.auth-input-label {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-bottom: 1.25rem;
  font-size: 0.875rem;
  color: #cbd5e1;
}

.auth-input-label input {
  background: #0f172a;
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 8px;
  padding: 0.75rem 1rem;
  color: #fff;
  font-size: 1rem;
  outline: none;
  transition: border-color 0.2s;
}

.auth-input-label input:focus {
  border-color: #38bdf8;
}

.auth-error-msg {
  color: #f87171;
  font-size: 0.85rem;
  margin: -0.5rem 0 1rem 0;
}

.full-width {
  width: 100%;
  padding: 0.75rem 1rem;
  font-size: 1rem;
  font-weight: 600;
  border-radius: 8px;
}
</style>
