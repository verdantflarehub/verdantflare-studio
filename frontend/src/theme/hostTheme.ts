import { readonly, ref } from 'vue'
import { createThemePreference, themeKey } from './preference'
import type { Theme, ThemeMode } from './preference'

const mode = ref<ThemeMode>('light'), actual = ref<Theme>('light'), warning = ref('')
const media = typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: dark)') : null
const preference = createThemePreference({
  read: () => localStorage.getItem(themeKey), write: value => localStorage.setItem(themeKey, value), dark: () => media?.matches ?? false,
  apply(value, resolved, remembered) {
    mode.value = value; actual.value = resolved
    document.documentElement.dataset.theme = resolved
    document.documentElement.dataset.mode = value
    document.body.dataset.theme = resolved
    warning.value = remembered ? '' : '主题已切换，暂无法记住此选择'
  },
})
const systemChanged = () => preference.systemChanged()
const storageChanged = (event: StorageEvent) => { if (event.key === themeKey || event.key === null) preference.storageChanged(event.newValue) }
media?.addEventListener('change', systemChanged)
window.addEventListener('storage', storageChanged)
if (import.meta.hot) import.meta.hot.dispose(() => { media?.removeEventListener('change', systemChanged); window.removeEventListener('storage', storageChanged) })
export const hostTheme = { mode: readonly(mode), actual: readonly(actual), warning: readonly(warning), select: preference.select }
