<script setup lang="ts">
import { onMounted } from 'vue'
import { useHostStore } from '../platform/hostStore'
import { request } from '../platform/client'

const hostStore = useHostStore()

onMounted(() => {
  const dialog = document.getElementById('loginDialog') as HTMLDialogElement | null
  const form = document.getElementById('loginForm') as HTMLFormElement | null
  if (!dialog || !form) return

  dialog.addEventListener('cancel', e => e.preventDefault())
  form.addEventListener('submit', async e => {
    e.preventDefault()
    const submitBtn = form.querySelector('button[type="submit"]') as HTMLButtonElement | null
    const errEl = document.getElementById('loginError')
    if (submitBtn) submitBtn.disabled = true
    if (errEl) errEl.textContent = ''
    try {
      const u = (form.elements.namedItem('username') as HTMLInputElement)?.value || ''
      const p = (form.elements.namedItem('password') as HTMLInputElement)?.value || ''
      const res = await request({
        path: 'login',
        method: 'POST',
        body: { username: u, password: p }
      })
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}))
        const codeMap: Record<string, string> = {
          ACCOUNT_LOCKED: '登录失败次数过多，请稍后再试',
          UNAUTHENTICATED: '用户名或密码不正确',
          SERVICE_UNAVAILABLE: 'Station 暂时无法连接'
        }
        throw new Error(codeMap[errData.code] || errData.message || '登录失败')
      }
      const passInput = form.elements.namedItem('password') as HTMLInputElement | null
      if (passInput) passInput.value = ''
      dialog.close()
      await hostStore.init()
      window.dispatchEvent(new CustomEvent('studio:login-success'))
    } catch (err: any) {
      if (errEl) errEl.textContent = err.message || '登录失败'
    } finally {
      if (submitBtn) submitBtn.disabled = false
    }
  })
})
</script>

<template>
  <dialog id="loginDialog" aria-labelledby="loginTitle">
    <form id="loginForm">
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
          <input id="loginUsername" name="username" autocomplete="username" required value="admin" placeholder="输入用户名">
        </div>
      </div>

      <div class="form-group">
        <label for="loginPassword">访问密码</label>
        <div class="input-wrap">
          <input id="loginPassword" name="password" type="password" autocomplete="current-password" required placeholder="输入访问凭据密码">
        </div>
      </div>

      <p id="loginError" role="alert"></p>

      <button class="btn primary submit-btn" type="submit">
        <span>连接并进入工作区</span>
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="9 18 15 12 9 6"></polyline>
        </svg>
      </button>
    </form>
  </dialog>
</template>
