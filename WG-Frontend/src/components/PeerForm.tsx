// PeerForm: tambah/ubah peer (F-3.1–F-3.5, F-3.8). IP kosong = otomatis (IP kosong pertama). Opsi "tempel public key"
// membuat peer tanpa private key di server. Galat 422 tampil per field; 409 tampil sebagai pesan form.
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { ApiError, apiFetch, jsonBody, type Interface, type Peer } from '../api/client'
import { splitList } from '../lib/peer-status'
import { Button } from './ui/button'
import { Dialog } from './ui/dialog'
import { Field } from './ui/input'
import { formatWita, formatGiB } from '../lib/format'

type FormState = { interface_id: string; name: string; note: string; address: string; public_key: string
  use_psk: boolean; client_allowed_ips: string; site_subnets: string; paste_key: boolean
  expires_at: string; quota_bytes: string; quota_period: string }

/** PeerForm: `current` null = tambah. Ganti interface/key/PSK tidak tersedia saat ubah (rotasi = Fase 12). */
export function PeerForm({ current, interfaces, onClose }: { current: Peer | null; interfaces: Interface[]; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>({
    interface_id: String(current?.interface_id ?? interfaces[0]?.id ?? ''), name: current?.name ?? '', note: current?.note ?? '', address: current?.address ?? '', public_key: '', use_psk: true, paste_key: false,
    client_allowed_ips: current?.client_allowed_ips.join(', ') ?? '', site_subnets: current?.site_subnets.join(', ') ?? '',
    expires_at: current?.expires_at ? formatWita(current.expires_at) : '', quota_bytes: current?.quota_bytes ? formatGiB(current.quota_bytes) : '', quota_period: current?.quota_period ?? 'none' })
  const [error, setError] = useState<ApiError | null>(null)

  const save = useMutation({
    mutationFn: () => {
      const body = {
        name: form.name,
        note: form.note,
        address: form.address.trim(),
        interface_id: Number(form.interface_id),
        use_psk: form.use_psk,
        public_key: form.paste_key ? form.public_key.trim() : '',
        expires_at: form.expires_at ? Math.floor(new Date(form.expires_at).getTime() / 1000) : null,
        quota_bytes: form.quota_bytes ? Math.round(parseFloat(form.quota_bytes) * 1024 * 1024 * 1024) : null,
        quota_period: form.quota_period,
        client_allowed_ips: splitList(form.client_allowed_ips),
        site_subnets: splitList(form.site_subnets),
      }
      const options = current ? { method: 'PUT', ...jsonBody(body) } : { method: 'POST', ...jsonBody(body) }
      return current ? apiFetch<Peer>(`/peers/${current.id}`, options) : apiFetch<Peer>('/peers', options)
    },
    onSuccess: async () => {
      await Promise.all([queryClient.invalidateQueries({ queryKey: ['peers'] }), queryClient.invalidateQueries({ queryKey: ['status'] })])
      onClose()
    },
    onError: (caught) => setError(caught instanceof ApiError ? caught : new ApiError('unknown', caught.message, 0)),
  })

  const text = (key: 'name' | 'note' | 'address' | 'public_key' | 'client_allowed_ips' | 'site_subnets') => ({
    value: form[key], error: error?.fieldMessage(key),
    onChange: (event: { target: { value: string } }) => setForm({ ...form, [key]: event.target.value }),
  })

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }} title={current ? `Ubah ${current.name}` : 'Tambah peer'}
      description="Perubahan langsung diterapkan ke kernel; bila kernel menolak, tidak ada yang tersimpan.">
      <form className="grid gap-4" onSubmit={(event) => { event.preventDefault(); setError(null); save.mutate() }}>
        {!current ? (
          <div className="grid gap-1.5">
            <label htmlFor="peer-interface" className="text-sm font-medium">Interface</label>
            <select id="peer-interface" value={form.interface_id} className="h-10 rounded-control border border-border bg-card px-3"
              onChange={(event) => setForm({ ...form, interface_id: event.target.value })}>
              {interfaces.map((row) => <option key={row.id} value={row.id}>{row.name} ({row.address})</option>)}
            </select>
          </div>
        ) : null}
        <Field id="peer-name" label="Nama" placeholder="laptop-bu-ani" {...text('name')} />
        <Field id="peer-note" label="Catatan" {...text('note')} />
        <Field id="peer-address" label="IP di tunnel" placeholder="kosong = otomatis" hint="Kosongkan untuk IP kosong pertama."
          {...text('address')} />
        <div className="grid gap-1.5">
          <label htmlFor="peer-expires" className="text-sm font-medium">Masa berlaku (tanggal kedaluwarsa)</label>
          <input id="peer-expires" type="datetime-local" value={form.expires_at}
            onChange={(event) => setForm({ ...form, expires_at: event.target.value })} />
          <p className="text-xs text-muted-foreground">ISO-8601 WITA (asia/makassar) → unix UTC. Kosong = tanpa batas.</p>
        </div>
        <div className="grid gap-1.5">
          <label htmlFor="peer-quota" className="text-sm font-medium">Kuota trafik (GiB)</label>
          <input id="peer-quota" type="number" min="0" step="1" value={form.quota_bytes}
            onChange={(event) => setForm({ ...form, quota_bytes: event.target.value })} />
          <p className="text-xs text-muted-foreground">GiB × 1024³ = byte. Kosong = tanpa kuota.</p>
        </div>
        <Field id="peer-period" label="Periode kuota">
          <select value={form.quota_period} onChange={(event) => setForm({ ...form, quota_period: event.target.value })}>
            <option value="none">Tidak ada</option>
            <option value="monthly">Bulan</option>
          </select>
        </Field>
        {!current ? (
          <>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.paste_key} onChange={(event) => setForm({ ...form, paste_key: event.target.checked })} />
              Tempel public key dari perangkat (private key tidak disimpan di server)
            </label>
            {form.paste_key ? <Field id="peer-public-key" label="Public key" className="font-mono" {...text('public_key')} /> : null}
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.use_psk} onChange={(event) => setForm({ ...form, use_psk: event.target.checked })} />
              Pakai preshared key (disarankan)
            </label>
          </>
        ) : null}
        <Field id="peer-allowed" label="AllowedIPs client" hint="Kosong = ikut interface. Dipisah koma." {...text('client_allowed_ips')} />
        <Field id="peer-sites" label="Subnet LAN di belakang peer (site-to-side)" hint="Opsional, mis. 192.168.50.0/24."
          {...text('site_subnets')} />
        {error && error.fields.length === 0 ? <p role="alert" className="text-sm text-destructive">{error.message}</p> : null}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose}>Batal</Button>
          <Button type="submit" disabled={save.isPending || (!current && interfaces.length === 0)}>
            {save.isPending ? 'Menyimpan…' : 'Simpan'}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
