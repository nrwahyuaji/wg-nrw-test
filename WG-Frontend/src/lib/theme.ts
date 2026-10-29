// Tema terang/gelap: pilihan tersimpan di localStorage ("light"/"dark"); tanpa pilihan mengikuti sistem
// (index.css menangani prefers-color-scheme). Kelas dipasang di <html>.
export type Theme = 'light' | 'dark' | 'system'

const KEY = 'wgpanel-theme'

/** currentTheme membaca pilihan tersimpan; nilai tidak dikenal → system. */
export function currentTheme(): Theme {
  const stored = localStorage.getItem(KEY)
  return stored === 'light' || stored === 'dark' ? stored : 'system'
}

/** applyTheme memasang kelas light/dark di <html> dan menyimpan pilihan. */
export function applyTheme(theme: Theme): void {
  const root = document.documentElement
  root.classList.toggle('dark', theme === 'dark')
  root.classList.toggle('light', theme === 'light')
  if (theme === 'system') localStorage.removeItem(KEY)
  else localStorage.setItem(KEY, theme)
}
