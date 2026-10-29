// Detail satu peer: identitas, status live, kunci publik (bisa disalin), dan aksi config/QR. Private key tidak pernah
// ditampilkan; hanya penanda apakah server menyimpannya.
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, QrCode } from 'lucide-react'
import { useState } from 'react'
import { Link, useParams } from 'react-router'
import { apiFetch, type Interface, type Peer } from '../api/client'
import { QrDialog } from '../components/QrDialog'
import { StatusBadge } from '../components/StatusBadge'
import { TrafficChart } from '../components/TrafficChart'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { CopyButton } from '../components/ui/copy-button'
import { useMe } from '../hooks/useMe'
import { livePolling, useStatus } from '../hooks/useStatus'
import { formatBytes, formatWita, relativeTime } from '../lib/format'
import { indexStatus } from '../lib/peer-status'

export default function PeerDetail() {
  const { id } = useParams()
  const me = useMe()
  const peer = useQuery({ queryKey: ['peers', id], queryFn: () => apiFetch<Peer>(`/peers/${id}`), ...livePolling })
  const interfaces = useQuery({ queryKey: ['interfaces'], queryFn: () => apiFetch<Interface[]>('/interfaces') })
  const status = useStatus()
  const [showQr, setShowQr] = useState(false)

  if (peer.isPending) return <p className="text-muted-foreground">Memuat…</p>
  if (peer.isError) return <p className="text-destructive">{peer.error.message}</p>
  const data = peer.data
  const live = indexStatus(status.data).get(data.id)
  const iface = interfaces.data?.find((row) => row.id === data.interface_id)

  const rows: [string, React.ReactNode][] = [
    ['Interface', <span className="font-mono">{iface?.name ?? data.interface_id}</span>],
    ['IP di tunnel', <span className="font-mono">{data.address}</span>],
    ['Status', <StatusBadge peer={data} live={live} />],
    ['Handshake terakhir', live?.last_handshake ? `${relativeTime(live.last_handshake)} (${formatWita(live.last_handshake)})` : 'belum pernah'],
    ['Endpoint terakhir', <span className="font-mono">{live?.endpoint || '—'}</span>],
    ['Terima / kirim (sejak interface dibuat)', <span className="font-mono">{formatBytes(live?.rx ?? 0)} / {formatBytes(live?.tx ?? 0)}</span>],
    ['Public key', <span className="flex min-w-0 items-center gap-1"><span className="truncate font-mono">{data.public_key}</span>
      <CopyButton value={data.public_key} label="Salin public key" /></span>],
    ['Private key di server', data.has_private_key ? 'ya (terenkripsi)' : 'tidak (ditempel dari perangkat)'],
    ['Preshared key', data.has_psk ? 'ya' : 'tidak'],
    ['AllowedIPs client', <span className="font-mono">{data.client_allowed_ips.join(', ') || `ikut interface (${iface?.client_allowed_ips.join(', ') ?? '—'})`}</span>],
    ['Subnet LAN (site-to-site)', <span className="font-mono">{data.site_subnets.join(', ') || '—'}</span>],
    ['Catatan', data.note || '—'],
    ['Dibuat', formatWita(data.created_at)],
  ]

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-6">
      <Link to="/peers" className="inline-flex items-center gap-2 text-sm text-primary hover:underline">
        <ArrowLeft size={16} aria-hidden /> Kembali ke daftar peer
      </Link>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold">{data.name}</h1>
        {me.data?.role === 'admin' ? <Button onClick={() => setShowQr(true)}><QrCode size={18} aria-hidden /> QR & config</Button> : null}
      </div>
      <Card>
        <dl className="grid gap-x-6 gap-y-3 sm:grid-cols-[12rem_minmax(0,1fr)]">
          {rows.map(([label, value]) => (
            <div key={label} className="contents">
              <dt className="text-sm text-muted-foreground">{label}</dt>
              <dd className="min-w-0">{value}</dd>
            </div>
          ))}
        </dl>
      </Card>
      <TrafficChart scope={`/peers/${data.id}`} />
      {showQr ? <QrDialog peer={data} onClose={() => setShowQr(false)} /> : null}
    </div>
  )
}
