// Dashboard (F-5.2): kartu total/aktif/online, ringkasan per interface + peringatan kesehatan (ip_forward_off dll.),
// peer yang segera kedaluwarsa, trafik hari ini (WITA) + top 5 peer, dan grafik trafik per interface. /dashboard ikut dipolling bersama /status (5 dtk, hanya saat tab terlihat).
import { useQueries, useQuery } from '@tanstack/react-query'
import { AlertTriangle } from 'lucide-react'
import { Link } from 'react-router'
import { apiFetch, type Dashboard as DashboardData, type InterfaceHealth } from '../api/client'
import { TrafficChart } from '../components/TrafficChart'
import { Badge } from '../components/ui/badge'
import { Card } from '../components/ui/card'
import { Cell, Table } from '../components/ui/table'
import { STATUS_POLL_MS } from '../hooks/useStatus'
import { formatBytes, relativeTime } from '../lib/format'

/** Teks peringatan kesehatan interface (kode dari /interfaces/{id}/health). */
export const HEALTH_WARNING: Record<string, string> = {
  not_in_kernel: 'Interface tidak ada di kernel. Jalankan ulang wgpanel-apply atau periksa log.',
  link_down: 'Link interface tidak UP.',
  port_mismatch: 'Port di kernel berbeda dari basis data.',
  peer_mismatch: 'Jumlah peer di kernel berbeda dari basis data.',
  ip_forward_off: 'Ada peer full-tunnel tetapi ip_forward host mati: client tidak bisa ke internet lewat VPN. Minta sysadmin mengaktifkan net.ipv4.ip_forward.',
  interface_disabled: 'Interface dinonaktifkan.',
}

/** Stat: satu kartu angka. */
function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <Card>
      <p className="text-sm text-muted-foreground">{label}</p>
      <p className="mt-1 text-3xl font-semibold tabular-nums">{value}</p>
    </Card>
  )
}

export default function Dashboard() {
  const dashboard = useQuery({
    queryKey: ['dashboard'], queryFn: () => apiFetch<DashboardData>('/dashboard'),
    refetchInterval: STATUS_POLL_MS, refetchIntervalInBackground: false,
  })
  const interfaces = dashboard.data?.interfaces ?? []
  const health = useQueries({
    queries: interfaces.map((row) => ({
      queryKey: ['health', row.id], queryFn: () => apiFetch<InterfaceHealth>(`/interfaces/${row.id}/health`),
      refetchInterval: 30_000, refetchIntervalInBackground: false,
    })),
  })

  if (dashboard.isPending) return <p className="text-muted-foreground">Memuat…</p>
  if (dashboard.isError) return <p className="text-destructive">{dashboard.error.message}</p>
  const data = dashboard.data
  const warnings = interfaces.flatMap((row, index) =>
    (health[index]?.data?.warnings ?? []).filter((code) => code !== 'interface_disabled').map((code) => ({ name: row.name, code })))

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-6">
      <h1 className="text-2xl font-semibold">Dashboard</h1>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Total peer" value={data.peers_total} />
        <Stat label="Peer aktif" value={data.peers_enabled} />
        <Stat label="Peer online" value={data.peers_online} />
        <Stat label="Trafik hari ini" value={formatBytes(data.today_rx + data.today_tx)} />
      </div>
      {warnings.length > 0 ? (
        <section aria-label="Peringatan" className="grid gap-2">
          {warnings.map(({ name, code }) => (
            <p key={`${name}-${code}`} role="alert" className="flex items-start gap-2 rounded-card border border-disabled bg-card p-3 text-sm">
              <AlertTriangle size={18} aria-hidden className="mt-0.5 shrink-0 text-disabled" />
              <span><span className="font-mono font-medium">{name}</span>: {HEALTH_WARNING[code] ?? code}</span>
            </p>
          ))}
        </section>
      ) : null}
      <Card title="Interface">
        <Table caption="Ringkasan interface" columns={['Nama', 'Status', 'Peer', 'Online']}
          empty={interfaces.length === 0 ? <p className="py-6 text-center text-muted-foreground">Belum ada interface. <Link className="text-primary underline" to="/interfaces">Buat interface</Link>.</p> : null}>
          {interfaces.map((row) => (
            <tr key={row.id}>
              <Cell mono>{row.name}</Cell>
              <Cell>{!row.enabled ? <Badge tone="disabled">Nonaktif</Badge> : row.up ? <Badge tone="online">Berjalan</Badge> : <Badge tone="danger">Tidak berjalan</Badge>}</Cell>
              <Cell mono>{row.peers}</Cell>
              <Cell mono>{row.peers_online}</Cell>
            </tr>
          ))}
        </Table>
      </Card>
      <Card title="Peer terboros hari ini">
        <Table caption="Lima peer dengan trafik terbesar sejak 00.00 WITA" columns={['Peer', 'Terima', 'Kirim']}
          empty={data.top_peers_today.length === 0 ? <p className="py-6 text-center text-muted-foreground">Belum ada trafik hari ini.</p> : null}>
          {data.top_peers_today.map((row) => (
            <tr key={row.id}>
              <Cell><Link to={`/peers/${row.id}`} className="text-primary hover:underline">{row.name}</Link></Cell>
              <Cell mono>{formatBytes(row.rx)}</Cell>
              <Cell mono>{formatBytes(row.tx)}</Cell>
            </tr>
          ))}
        </Table>
      </Card>
      {interfaces.filter((row) => row.enabled).map((row) => (
        <TrafficChart key={row.id} scope={`/interfaces/${row.id}`} title={`Trafik ${row.name}`} />
      ))}
      <Card title="Segera kedaluwarsa (7 hari)">
        {data.expiring_soon.length === 0 ? <p className="text-muted-foreground">Tidak ada.</p> : (
          <ul className="grid gap-2">
            {data.expiring_soon.map((row) => (
              <li key={row.id}><Link to={`/peers/${row.id}`} className="text-primary hover:underline">{row.name}</Link>
                <span className="text-muted-foreground"> — {relativeTime(row.expires_at)}</span></li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  )
}
