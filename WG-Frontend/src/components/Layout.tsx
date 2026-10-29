// Layout utama: sidebar gelap (menu disaring per role dari /me), kepala berisi nama user, pilih tema, dan logout.
// Di layar sempit sidebar menjadi baris menu di atas konten.
import { useQueryClient } from '@tanstack/react-query'
import { Gauge, LogOut, Network, Settings, ScrollText, UserCircle, Users, Waypoints } from 'lucide-react'
import { useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router'
import { apiFetch } from '../api/client'
import { useMe } from '../hooks/useMe'
import { applyTheme, currentTheme, type Theme } from '../lib/theme'
import { Button } from './ui/button'
import { cn } from './ui/cn'

type Role = 'admin' | 'operator' | 'user'

/** Menu sidebar (PLAN 6.3). `roles` = role yang boleh melihatnya (DESIGN 7.4). */
const MENU: { to: string; label: string; icon: typeof Gauge; roles: Role[] }[] = [
  { to: '/', label: 'Dashboard', icon: Gauge, roles: ['admin', 'operator'] },
  { to: '/interfaces', label: 'Interface', icon: Network, roles: ['admin', 'operator'] },
  { to: '/peers', label: 'Peer', icon: Waypoints, roles: ['admin', 'operator'] },
  { to: '/users', label: 'User', icon: Users, roles: ['admin'] },
  { to: '/audit', label: 'Audit', icon: ScrollText, roles: ['admin'] },
  { to: '/settings', label: 'Pengaturan', icon: Settings, roles: ['admin'] },
  { to: '/account', label: 'Akun', icon: UserCircle, roles: ['admin', 'operator', 'user'] },
]

/** Label role yang ditampilkan di kepala halaman. */
const ROLE_LABEL: Record<string, string> = { admin: 'Admin', operator: 'Operator', user: 'User' }

/** menuFor menyaring menu sesuai role; role tak dikenal hanya melihat Akun. */
export function menuFor(role: string | undefined) {
  return MENU.filter((item) => item.roles.includes(role as Role))
}

export function Layout() {
  const me = useMe()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [theme, setTheme] = useState<Theme>(currentTheme())

  if (me.isPending) return <p className="p-6 text-muted-foreground">Memuat…</p>
  if (me.isError) return <p className="p-6 text-destructive">Gagal memuat profil: {me.error.message}</p>

  // logout mencabut sesi di server, membuang cache query, lalu ke /login.
  const logout = async () => {
    await apiFetch('/auth/logout', { method: 'POST' }).catch(() => undefined)
    queryClient.clear()
    navigate('/login', { replace: true })
  }

  return (
    <div className="min-h-screen bg-background text-foreground font-sans md:grid md:grid-cols-[15rem_1fr]">
      <aside className="bg-sidebar text-[#e5edf9] md:min-h-screen">
        <div className="px-5 py-4 text-lg font-semibold">WG-WebUI</div>
        <nav aria-label="Menu utama" className="flex gap-1 overflow-x-auto px-3 pb-3 md:flex-col md:overflow-visible">
          {menuFor(me.data.role).map(({ to, label, icon: Icon }) => (
            <NavLink key={to} to={to} end={to === '/'}
              className={({ isActive }) => cn(
                'flex shrink-0 items-center gap-3 rounded-control px-3 py-2 text-sm',
                isActive ? 'bg-white/15 font-semibold' : 'hover:bg-white/10',
              )}>
              <Icon size={18} aria-hidden /> {label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <div className="min-w-0">
        <header className="flex flex-wrap items-center justify-end gap-3 border-b border-border bg-card px-6 py-3">
          <label className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">Tema</span>
            <select value={theme} className="h-8 rounded-control border border-border bg-card px-2"
              onChange={(event) => { const next = event.target.value as Theme; applyTheme(next); setTheme(next) }}>
              <option value="system">Ikut sistem</option>
              <option value="light">Terang</option>
              <option value="dark">Gelap</option>
            </select>
          </label>
          <span className="flex items-center gap-2 text-sm"><span className="font-medium">{me.data.username}</span>
            <span className="rounded-badge border border-border px-1.5 py-0.5 text-xs text-muted-foreground">{ROLE_LABEL[me.data.role] ?? me.data.role}</span></span>
          <Button variant="outline" size="sm" onClick={logout}><LogOut size={16} aria-hidden /> Keluar</Button>
        </header>
        <main className="p-6"><Outlet /></main>
      </div>
    </div>
  )
}
