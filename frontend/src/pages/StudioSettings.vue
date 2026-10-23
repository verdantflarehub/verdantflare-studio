<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useHostStore } from '../platform/hostStore'
import {
  NSFW_MAX_TTL_MS,
  formatClockTime,
  formatExpiresAt,
  formatRemainingTime,
  hostContentPreferences,
} from '../platform/contentPreferences'
import type { ContentPreferencesState } from '../platform/contentPreferences'

const host = useHostStore()
const prefState = ref<ContentPreferencesState>(hostContentPreferences.get())
const expiredAtMs = ref<number | null>(null)
const announcement = ref('')
const enableDialog = ref<HTMLDialogElement>()
const switchButton = ref<HTMLButtonElement>()
const cancelEnableButton = ref<HTMLButtonElement>()
const proposedExpiresAt = ref('')

let unsubscribe: (() => void) | null = null
let ticker: ReturnType<typeof setInterval> | null = null
let soonAnnounced = false

const isLoading = computed(() => host.sessionStatus === 'loading')
const isDisabled = computed(() => !host.connected || host.sessionStatus === 'error' || !prefState.value.storageAvailable)
const remainingText = computed(() => formatRemainingTime(prefState.value.remainingMs))
const expiresAtText = computed(() => prefState.value.expiresAt ? formatExpiresAt(prefState.value.expiresAt) : '')
const expiredClockText = computed(() => expiredAtMs.value ? formatClockTime(expiredAtMs.value) : '')

function showToast(text: string) {
  const el = document.getElementById('toast')
  if (!el) return
  el.textContent = text
  el.classList.add('visible')
  setTimeout(() => {
    if (el.textContent === text) el.classList.remove('visible')
  }, 3200)
}

function refreshPref() {
  prefState.value = hostContentPreferences.get()
  if (prefState.value.includeNsfw && prefState.value.expiringSoon && !soonAnnounced) {
    soonAnnounced = true
    announcement.value = 'NSFW 显示即将自动关闭'
  }
}

function onSwitchClick() {
  if (isDisabled.value) return
  if (prefState.value.includeNsfw) {
    disableNow()
  } else {
    openEnableModal()
  }
}

function openEnableModal() {
  proposedExpiresAt.value = formatExpiresAt(Date.now() + NSFW_MAX_TTL_MS)
  enableDialog.value?.showModal()
  cancelEnableButton.value?.focus()
}

function closeEnableModal() {
  enableDialog.value?.close()
  switchButton.value?.focus()
}

function confirmEnable() {
  try {
    expiredAtMs.value = null
    soonAnnounced = false
    prefState.value = hostContentPreferences.enable()
    enableDialog.value?.close()
    announcement.value = 'NSFW 显示已开启'
    showToast('已开启 NSFW 显示，24 小时后自动关闭')
    switchButton.value?.focus()
  } catch {
    enableDialog.value?.close()
    showToast('当前浏览器无法保存本机设置，暂不能开启')
    refreshPref()
  }
}

function disableNow() {
  soonAnnounced = false
  prefState.value = hostContentPreferences.disable('manual')
  announcement.value = 'NSFW 显示已关闭'
  showToast('已关闭，NSFW 内容已隐藏')
  switchButton.value?.focus()
}

onMounted(() => {
  refreshPref()
  unsubscribe = hostContentPreferences.subscribe((next, reason) => {
    prefState.value = next
    if (!next.includeNsfw) {
      soonAnnounced = false
      if (reason === 'expired') {
        expiredAtMs.value = Date.now()
        announcement.value = 'NSFW 显示已到期自动关闭'
        showToast('NSFW 显示已到期自动关闭，相关内容已隐藏')
      } else if (reason === 'storage') {
        announcement.value = 'NSFW 显示已关闭'
        showToast('另一个标签页关闭了 NSFW 显示，相关内容已隐藏')
      }
    } else {
      expiredAtMs.value = null
    }
  })
  ticker = setInterval(() => {
    hostContentPreferences.checkAndSync('expired')
    refreshPref()
  }, 15_000)
})

onBeforeUnmount(() => {
  unsubscribe?.()
  if (ticker) clearInterval(ticker)
})
</script>

<template>
  <main class="main settings-main">
    <header class="page-heading">
      <h1>设置</h1>
      <p>这些设置只保存在当前浏览器，不会同步到账号或其他设备。</p>
    </header>

    <section class="settings-card" aria-labelledby="settings-content-group-title">
      <header id="settings-content-group-title">内容显示</header>

      <div v-if="isLoading" class="setting-row" aria-busy="true">
        <div>
          <div class="skeleton" style="width: 140px" />
          <div class="skeleton" style="width: 320px" />
        </div>
        <div class="skeleton" style="width: 44px; height: 24px; border-radius: 999px" />
      </div>

      <template v-else>
        <div class="setting-row">
          <div>
            <h2 id="nsfw-label">显示 NSFW 内容</h2>
            <p id="nsfw-desc">显示被标记为 18+ 的产物，仅本设备，24 小时后自动关闭。</p>
          </div>
          <button
            ref="switchButton"
            id="nsfw-switch"
            class="switch"
            :class="{ soon: prefState.includeNsfw && prefState.expiringSoon }"
            role="switch"
            :aria-checked="prefState.includeNsfw ? 'true' : 'false'"
            aria-labelledby="nsfw-label"
            aria-describedby="nsfw-desc"
            :disabled="isDisabled"
            @click="onSwitchClick"
          />
        </div>

        <div v-if="!prefState.storageAvailable" class="notice error" role="alert">
          当前浏览器无法保存本机设置，暂不能开启。请允许本站使用本地存储后刷新页面。
        </div>
        <div v-else-if="host.sessionStatus === 'error'" class="notice error" role="alert">
          当前身份读取失败，暂不能修改本机显示设置。
        </div>
        <div
          v-else-if="prefState.includeNsfw"
          id="status-area"
          class="status-area"
          :class="{ soon: prefState.expiringSoon }"
        >
          <div>
            <div class="remain tabular" data-remaining>{{ remainingText }}</div>
            <div class="remain-sub tabular">
              将于 {{ expiresAtText }} 自动关闭 · 不能延长 · 退出登录或切换账号后立即关闭
              <span v-if="prefState.expiringSoon" id="soon-label" class="soon-text"> · 即将自动关闭</span>
            </div>
          </div>
          <button id="close-now" class="button" @click="disableNow">立即关闭</button>
        </div>
        <div v-else-if="expiredAtMs" class="notice" role="status">
          已于 {{ expiredClockText }} 到期自动关闭，可重新开启。
        </div>
      </template>

      <div class="settings-foot">
        NSFW 内容由 AI 按评估规范自动标记。关闭时这些内容在列表、计数与预览中完全不出现。本开关只影响显示，不改变任何权限。
      </div>
    </section>

    <div class="sr-only" role="status" aria-live="polite">{{ announcement }}</div>

    <Teleport to="body">
      <dialog ref="enableDialog" class="settings-dialog" aria-labelledby="nsfw-enable-dialog-title" @close="switchButton?.focus()">
        <div class="dialog-head">
          <h2 id="nsfw-enable-dialog-title">显示 18+ 内容？</h2>
          <button class="small-button" aria-label="关闭" @click="closeEnableModal">关闭</button>
        </div>
        <p>开启后，当前浏览器中的 Studio 页面会显示被标记为 NSFW（18+）的产物，并带有 18+ 标识。</p>
        <p>仅对本设备生效，不会同步到账号；同一账号在其他设备上仍为关闭。</p>
        <p>
          将于 <strong class="tabular">{{ proposedExpiresAt }}</strong> 自动关闭（24 小时），不能延长。退出登录、切换账号或组织后立即关闭。
        </p>
        <p>请确认你已年满 18 周岁，并在合适的环境中查看。</p>
        <div class="dialog-actions">
          <button ref="cancelEnableButton" id="enable-cancel" class="button" @click="closeEnableModal">取消</button>
          <button id="enable-ok" class="button primary" @click="confirmEnable">开启 24 小时</button>
        </div>
      </dialog>
    </Teleport>
  </main>
</template>

<style scoped>
.settings-main { padding: 36px 44px 28px; min-width: 0; display: flex; flex-direction: column; }
.page-heading { margin-bottom: 24px; }
.page-heading h1 { font-size: 28px; line-height: 1.28; letter-spacing: -0.8px; font-weight: 650; margin: 0 0 8px; }
.page-heading p { margin: 0; color: var(--muted); }
.tabular { font-variant-numeric: tabular-nums; font-feature-settings: "tnum" 1; }
.settings-card { max-width: 760px; border-radius: 12px; background: var(--shell-panel); box-shadow: var(--vf-shadow-card); overflow: hidden; }
.settings-card > header { padding: 14px 20px; border-bottom: 0.5px solid var(--line, var(--vf-hairline-outer)); font-size: 13px; font-weight: 650; }
.setting-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 20px; align-items: center; padding: 18px 20px; }
.setting-row h2 { font-size: 14px; font-weight: 600; margin: 0 0 4px; }
.setting-row p { margin: 0; font-size: 12px; color: var(--muted); }
.switch { position: relative; width: 44px; height: 24px; border-radius: 999px; background: var(--line-strong, rgba(120, 130, 125, .35)); transition: background-color 180ms ease; flex: none; }
.switch::after { content: ""; position: absolute; top: 3px; left: 3px; width: 18px; height: 18px; border-radius: 50%; background: #fff; box-shadow: 0 1px 3px rgba(0, 0, 0, .25); transition: transform 220ms ease; }
.switch[aria-checked="true"] { background: var(--accent); }
.switch[aria-checked="true"]::after { transform: translateX(20px); }
.switch.soon[aria-checked="true"] { background: var(--amber, #b58032); }
.status-area { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 16px; align-items: center; margin: 0 20px 18px; padding: 12px 14px; border-radius: 9px; background: var(--selected, rgba(8, 126, 96, .09)); box-shadow: inset 0 0 0 0.5px rgba(8, 126, 96, .22); }
.status-area.soon { background: var(--amber-soft, rgba(181, 128, 50, .13)); box-shadow: inset 0 0 0 0.5px rgba(181, 128, 50, .32); }
.remain { font-size: 15px; font-weight: 600; color: var(--accent); }
.status-area.soon .remain, .soon-text { color: var(--amber, #b58032); font-weight: 600; }
.remain-sub { font-size: 11px; color: var(--muted); margin-top: 2px; }
.notice { margin: 0 20px 18px; padding: 10px 14px; border-radius: 9px; font-size: 12px; background: var(--shell-button); color: var(--ink); }
.notice.error { background: var(--danger-soft, rgba(201, 87, 67, .12)); color: var(--danger, #c95743); }
.settings-foot { padding: 14px 20px; border-top: 0.5px solid var(--line, var(--vf-hairline-outer)); font-size: 11px; color: var(--muted); line-height: 1.7; }
.button { border: 1px solid var(--line-strong, var(--vf-hairline-outer)); border-radius: 7px; padding: 7px 14px; font-size: 12px; font-weight: 550; background: transparent; color: var(--ink); }
.button:hover { background: var(--shell-button); }
.button.primary { background: var(--accent); color: var(--button-ink, #fff); border-color: var(--accent); }
.settings-dialog { width: min(500px, calc(100vw - 32px)); padding: 26px; border: 0; border-radius: 14px; background: var(--panel); color: var(--ink); box-shadow: var(--vf-shadow-modal, var(--vf-shadow-popover)); }
.settings-dialog::backdrop { background: rgba(17, 19, 16, .36); backdrop-filter: blur(3px); }
.dialog-head { display: flex; justify-content: space-between; align-items: center; gap: 20px; margin-bottom: 14px; }
.dialog-head h2 { margin: 0; font-size: 19px; font-weight: 650; }
.settings-dialog p { font-size: 13px; margin: 8px 0; line-height: 1.65; color: var(--muted); }
.dialog-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 22px; }
.small-button { border: 0.5px solid var(--vf-hairline-outer); border-radius: 5px; padding: 5px 10px; font-size: 12px; background: var(--shell-button); color: var(--ink); }
.sr-only { position: absolute; width: 1px; height: 1px; clip-path: inset(50%); overflow: hidden; white-space: nowrap; }
</style>
