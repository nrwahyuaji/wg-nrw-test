// Badge status peer: teks + warna (warna bukan satu-satunya penanda, aksesibel untuk buta warna).
import type { Peer, PeerStatus } from '../api/client'
import { peerDisplayStatus } from '../lib/peer-status'
import { Badge } from './ui/badge'

export function StatusBadge({ peer, live }: { peer: Pick<Peer, 'enabled' | 'disabled_reason'>; live?: PeerStatus }) {
  const status = peerDisplayStatus(peer, live)
  return <Badge tone={status.tone}>{status.label}</Badge>
}
