/**
 * VerdantFlare Studio Mock Configuration & Trigger Controller
 * 
 * Mocks enabling conditions (evaluated in order of priority):
 * 1. URL Query parameter: `?mock=true` (forces ON) or `?mock=false` (forces OFF)
 * 2. LocalStorage override: `localStorage.getItem('vf_enable_mocks') === 'true'`
 * 3. Vite environment variable: `import.meta.env.VITE_ENABLE_MOCKS === 'true'`
 * 
 * Default is FALSE (production/live clean state).
 */

export function isMockEnabled(): boolean {
  if (typeof window === 'undefined') return false

  // 1. URL search or hash search parameter (highest priority for testing & demo)
  try {
    const searchParams = new URLSearchParams(window.location.search)
    if (searchParams.get('mock') === 'true') return true
    if (searchParams.get('mock') === 'false') return false

    // Also check hash query params like #/market?mock=true
    const hashIndex = window.location.hash.indexOf('?')
    if (hashIndex !== -1) {
      const hashParams = new URLSearchParams(window.location.hash.slice(hashIndex))
      if (hashParams.get('mock') === 'true') return true
      if (hashParams.get('mock') === 'false') return false
    }
  } catch {}

  // 2. LocalStorage setting
  try {
    const stored = localStorage.getItem('vf_enable_mocks')
    if (stored === 'true') return true
    if (stored === 'false') return false
  } catch {}

  // 3. Vite environment variable
  try {
    return import.meta.env.VITE_ENABLE_MOCKS === 'true'
  } catch {
    return false
  }
}

/**
 * Enable mocks in local storage and reload or notify.
 */
export function enableMocks(reload = true) {
  try {
    localStorage.setItem('vf_enable_mocks', 'true')
    console.info('[VF Mocks] Enabled mocks in localStorage.')
    if (reload && typeof window !== 'undefined') {
      window.location.reload()
    }
  } catch {}
}

/**
 * Disable mocks in local storage.
 */
export function disableMocks(reload = true) {
  try {
    localStorage.setItem('vf_enable_mocks', 'false')
    console.info('[VF Mocks] Disabled mocks in localStorage.')
    if (reload && typeof window !== 'undefined') {
      window.location.reload()
    }
  } catch {}
}

// Attach helper to window for easy developer toggle in browser DevTools console
if (typeof window !== 'undefined') {
  (window as any).__VF_MOCKS__ = {
    isEnabled: isMockEnabled,
    enable: () => enableMocks(true),
    disable: () => disableMocks(true)
  }
}
