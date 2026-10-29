// Titik masuk SPA: pasang tema tersimpan lalu render App (router + TanStack Query) ke #root.
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App'
import { applyTheme, currentTheme } from './lib/theme'

applyTheme(currentTheme()) // pasang tema tersimpan sebelum render pertama (tanpa kedip)
const root = document.getElementById('root')
if (!root) throw new Error('elemen #root tidak ditemukan di index.html')
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
