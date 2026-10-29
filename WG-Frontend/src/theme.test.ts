// Uji token desain: index.css harus memuat setiap warna Plan/tailwind.theme.json untuk tema terang DAN gelap,
// dengan nilai yang sama persis. Mencegah token tersalin salah atau terlupa saat index.css diubah.
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import theme from './tailwind.theme.json'

const colors = theme.color as Record<string, { light: string; dark: string }>
const css = readFileSync(new URL('./index.css', import.meta.url), 'utf8')

describe('token desain', () => {
  it.each(Object.entries(colors))('%s ada di tema terang dan gelap', (name, value) => {
    expect(css).toContain(`--color-${name}: ${value.light};`)
    // dua salinan gelap: kelas .dark dan prefers-color-scheme
    expect(css.split(`--color-${name}: ${value.dark};`).length - 1).toBeGreaterThanOrEqual(2)
  })
})
