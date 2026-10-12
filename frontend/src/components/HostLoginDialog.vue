<script setup lang="ts">
import { ref, watch } from 'vue'
import { useHostStore } from '../platform/hostStore'
import { request } from '../platform/client'

const hostStore = useHostStore()
const dialog = ref<HTMLDialogElement | null>(null)
const username = ref(''), password = ref(''), error = ref(''), pending = ref(false)
watch([() => hostStore.loginOpen, dialog], ([open, element]) => {
  if (!element) return
  if (open && !element.open) element.showModal()
  else if (!open && element.open) element.close()
}, { flush: 'post' })
async function submit() {
  if (pending.value) return
  pending.value = true; error.value = ''
  try {
    const res = await request({ path: 'login', method: 'POST', body: { username: username.value, password: password.value } }, { realOnly: true })
    if (!res.ok) {
      const data = await res.json().catch(() => ({}))
      const labels: Record<string, string> = { ACCOUNT_LOCKED: '登录失败次数过多，请稍后再试', UNAUTHENTICATED: '用户名或密码不正确', SERVICE_UNAVAILABLE: 'Station 暂时无法连接' }
      error.value = labels[data.code] || '暂时无法登录，请稍后重试'
      return
    }
    password.value = ''
    await hostStore.init()
    if (!hostStore.connected) { error.value = '暂时无法恢复登录状态，请重试连接'; return }
    hostStore.loginOpen = false
    window.dispatchEvent(new CustomEvent('studio:login-success'))
  } catch { error.value = '暂时无法连接 Station，请稍后重试' }
  finally { pending.value = false }
}
</script>

<template>
  <dialog ref="dialog" id="loginDialog" aria-labelledby="loginTitle" @close="hostStore.loginOpen = false" @cancel="hostStore.loginOpen = false">
    <form id="loginForm" @submit.prevent="submit">
      <div class="login-brand-header">
        <div class="login-logo-badge">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="#087e60" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 2L2 7l10 5 10-5-10-5z"></path>
            <path d="M2 17l10 5 10-5"></path>
            <path d="M2 12l10 5 10-5"></path>
          </svg>
        </div>
        <div>
          <span class="eyebrow">VERDANTFLARE STATION</span>
          <h2 id="loginTitle">登录 Station</h2>
        </div>
      </div>
      <p class="login-subtitle">连接你的青焰创作工作区，调度 5090 集群算力与服务。</p>
      
      <div class="form-group">
        <label for="loginUsername">用户名</label>
        <div class="input-wrap">
          <input v-model="username" id="loginUsername" name="username" autocomplete="username" required placeholder="输入用户名">
        </div>
      </div>

      <div class="form-group">
        <label for="loginPassword">访问密码</label>
        <div class="input-wrap">
          <input v-model="password" id="loginPassword" name="password" type="password" autocomplete="current-password" required placeholder="输入访问凭据密码">
        </div>
      </div>

      <p id="loginError" role="alert">{{ error }}</p>
      <button v-if="hostStore.sessionStatus === 'error'" class="btn" type="button" @click="hostStore.init()">重试连接</button>

      <button class="btn primary submit-btn" type="submit" :disabled="pending">
        <span>{{ pending ? '正在连接…' : '连接并进入工作区' }}</span>
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="9 18 15 12 9 6"></polyline>
        </svg>
      </button>
    </form>
  </dialog>
</template>
