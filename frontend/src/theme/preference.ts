export type ThemeMode = 'light' | 'dark' | 'system'
export type Theme = 'light' | 'dark'
export const themeKey = 'vf_studio_theme'
export const themeLabels: Record<ThemeMode, string> = { light: '浅色', dark: '深色', system: '跟随系统' }
export function themeMode(value: unknown): ThemeMode { return value === 'dark' || value === 'system' ? value : 'light' }

/** One preference controls the host and every embedded page. */
export function createThemePreference(env: {
  read(): unknown; write(mode: ThemeMode): void; dark(): boolean
  apply(mode: ThemeMode, actual: Theme, remembered: boolean): void
}) {
  let mode: ThemeMode = 'light'
  try { mode = themeMode(env.read()) } catch { /* storage is optional */ }
  function apply(remembered = true) { env.apply(mode, mode === 'system' ? (env.dark() ? 'dark' : 'light') : mode, remembered) }
  apply()
  return {
    select(value: ThemeMode) { mode = themeMode(value); let remembered = true; try { env.write(mode) } catch { remembered = false }; apply(remembered) },
    systemChanged() { if (mode === 'system') apply() },
    storageChanged(value: unknown) { mode = themeMode(value); apply() },
  }
}
