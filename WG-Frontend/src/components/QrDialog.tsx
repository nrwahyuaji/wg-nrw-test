// QrDialog (F-4.1, F-4.2): QR dari endpoint /qr + tombol unduh .conf. Private key tidak pernah ditampilkan sebagai
// teks; ia hanya ada di dalam gambar QR dan berkas unduhan. Peer dari public key tempelan diberi peringatan.
import { Download } from 'lucide-react'
import type { Peer } from '../api/client'
import { Dialog } from './ui/dialog'

export function QrDialog({ peer, onClose }: { peer: Peer; onClose: () => void }) {
  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }} title={`Config ${peer.name}`}
      description="Pindai dengan aplikasi WireGuard, atau unduh berkas .conf. Jangan bagikan: berisi private key.">
      <div className="grid gap-4">
        {!peer.has_private_key ? (
          <p role="note" className="rounded-control border border-disabled p-3 text-sm text-disabled">
            Peer ini dibuat dari public key tempelan. Private key tidak disimpan di server: isi baris PrivateKey di perangkat.
          </p>
        ) : null}
        <img src={`/api/v1/peers/${peer.id}/qr`} alt={`Kode QR konfigurasi ${peer.name}`} width={256} height={256}
          className="mx-auto rounded-control bg-white p-2" />
        <a href={`/api/v1/peers/${peer.id}/config`} download
          className="inline-flex h-10 items-center justify-center gap-2 rounded-control bg-primary px-4 font-medium text-primary-foreground hover:opacity-90">
          <Download size={18} aria-hidden /> Unduh .conf
        </a>
      </div>
    </Dialog>
  )
}
