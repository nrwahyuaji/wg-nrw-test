// PeerTable: kolom nama, IP, status (online/offline/nonaktif + alasan), handshake relatif, rx/tx, dan aksi
// (PLAN 7.1). Status live digabung per peer_id dari useStatus; aksi tulis hanya untuk admin.
import { Link } from 'react-router'
import { QrCode, Pencil, Power, Trash2 } from 'lucide-react'
import type { Interface, Peer, PeerStatus } from '../api/client'
import { formatBytes, formatWita } from '../lib/format'
import { StatusBadge } from './StatusBadge'
import { Button } from './ui/button'
import { Cell, Table } from './ui/table'

export type PeerAction = 'qr' | 'edit' | 'toggle' | 'delete'

export function PeerTable({ peers, interfaces, status, isAdmin, onAction }: {
  peers: Peer[]
  interfaces: Interface[]
  status: Map<number, PeerStatus>
  isAdmin: boolean
  onAction: (action: PeerAction, peer: Peer) => void
}) {
  const interfaceName = new Map(interfaces.map((row) => [row.id, row.name]))
  return (
    <Table caption="Daftar peer WireGuard" columns={['Nama', 'Interface', 'IP', 'Status', 'Masa Berlaku', 'Kuota', 'Terima / kirim', 'Aksi']}
      empty={peers.length === 0 ? <p className="py-6 text-center text-muted-foreground">Belum ada peer.</p> : null}>
      {peers.map((peer) => {
        const live = status.get(peer.id)
        const expiryLabel = peer.expires_at ? formatWita(peer.expires_at) : '—'
        const quotaLabel = peer.quota_bytes && peer.quota_bytes > 0 ? formatBytes(peer.quota_bytes) : '—'
        return (
          <tr key={peer.id}>
            <Cell><Link to={`/peers/${peer.id}`} className="whitespace-nowrap font-medium text-primary hover:underline">{peer.name}</Link></Cell>
            <Cell mono>{interfaceName.get(peer.interface_id) ?? peer.interface_id}</Cell>
            <Cell mono>{peer.address}</Cell>
            <Cell><StatusBadge peer={peer} live={live} /></Cell>
            <Cell mono>{expiryLabel}</Cell>
            <Cell mono>{quotaLabel}</Cell>
            <Cell mono>{formatBytes(live?.rx ?? 0)} / {formatBytes(live?.tx ?? 0)}</Cell>
            <Cell>
              {isAdmin ? (
                <div className="flex gap-1">
                  <Button variant="ghost" size="sm" aria-label={`QR dan config ${peer.name}`} onClick={() => onAction('qr', peer)}>
                    <QrCode size={16} aria-hidden /></Button>
                  <Button variant="ghost" size="sm" aria-label={`Ubah ${peer.name}`} onClick={() => onAction('edit', peer)}>
                    <Pencil size={16} aria-hidden /></Button>
                  <Button variant="ghost" size="sm" aria-label={`${peer.enabled ? 'Nonaktifkan' : 'Aktifkan'} ${peer.name}`}
                    onClick={() => onAction('toggle', peer)}><Power size={16} aria-hidden /></Button>
                  <Button variant="ghost" size="sm" aria-label={`Hapus ${peer.name}`} onClick={() => onAction('delete', peer)}>
                    <Trash2 size={16} aria-hidden className="text-destructive" /></Button>
                </div>
              ) : null}
            </Cell>
          </tr>
        )
      })}
    </Table>
  )
}
