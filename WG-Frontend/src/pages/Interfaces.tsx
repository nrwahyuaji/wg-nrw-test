// Halaman interface (F-2): daftar, tambah, dan ubah. Galat 422 ditampilkan per field; 409 (nama/port dipakai) dan
// subnet_locked ditampilkan di form. Ganti subnet meminta konfirmasi ketik nama (DESIGN 11). Tulis = admin saja.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus } from 'lucide-react'
import { useState } from 'react'
import { ApiError, apiFetch, jsonBody, type Interface, type InterfaceRequest } from '../api/client'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { ConfirmDialog } from '../components/ui/confirm-dialog'
import { Dialog } from '../components/ui/dialog'
import { Field } from '../components/ui/input'
import { CopyButton } from '../components/ui/copy-button'
import { Cell, Table } from '../components/ui/table'
import { useMe } from '../hooks/useMe'

/** Isian form dalam bentuk teks; daftar dipisah koma. */
type FormState = { name: string; address: string; listen_port: string; mtu: string; endpoint_host: string
  client_dns: string; client_allowed_ips: string; client_keepalive: string }

const EMPTY: FormState = { name: '', address: '10.8.0.1/24', listen_port: '51820', mtu: '0', endpoint_host: '',
  client_dns: '1.1.1.1', client_allowed_ips: '0.0.0.0/0', client_keepalive: '25' }

/** toForm mengisi form dari interface yang ada (mode ubah). */
function toForm(row: Interface): FormState {
  return { name: row.name, address: row.address, listen_port: String(row.listen_port), mtu: String(row.mtu),
    endpoint_host: row.endpoint_host, client_dns: row.client_dns.join(', '),
    client_allowed_ips: row.client_allowed_ips.join(', '), client_keepalive: String(row.client_keepalive) }
}

/** toRequest mengubah teks form menjadi body API. Angka kosong → 0 agar validasi server yang menolak. */
export function toRequest(form: FormState): InterfaceRequest {
  const list = (raw: string) => raw.split(',').map((item) => item.trim()).filter(Boolean)
  return { name: form.name.trim(), address: form.address.trim(), listen_port: Number(form.listen_port) || 0,
    mtu: Number(form.mtu) || 0, endpoint_host: form.endpoint_host.trim(), client_dns: list(form.client_dns),
    client_allowed_ips: list(form.client_allowed_ips), client_keepalive: Number(form.client_keepalive) || 0 }
}

export default function Interfaces() {
  const me = useMe()
  const isAdmin = me.data?.role === 'admin'
  const list = useQuery({ queryKey: ['interfaces'], queryFn: () => apiFetch<Interface[]>('/interfaces') })
  const [editing, setEditing] = useState<Interface | 'new' | null>(null)

  return (
    // grid-cols-[minmax(0,1fr)]: tanpa ini lebar-min tabel melebarkan track grid sehingga halaman bergulir ke
    // samping di HP; dengan batas 0 tabel menggulir di dalam kartunya sendiri.
    <div className="grid grid-cols-[minmax(0,1fr)] gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold">Interface</h1>
        {isAdmin ? <Button onClick={() => setEditing('new')}><Plus size={18} aria-hidden /> Tambah interface</Button> : null}
      </div>
      <Card>
        {list.isPending ? <p className="text-muted-foreground">Memuat…</p>
          : list.isError ? <p className="text-destructive">{list.error.message}</p>
          : (
            <Table caption="Daftar interface WireGuard" columns={['Nama', 'Alamat', 'Port', 'Endpoint', 'Public key', 'Status', 'Aksi']}
              empty={list.data.length === 0 ? <p className="py-6 text-center text-muted-foreground">Belum ada interface.</p> : null}>
              {list.data.map((row) => (
                <tr key={row.id}>
                  <Cell mono>{row.name}</Cell>
                  <Cell mono>{row.address}</Cell>
                  <Cell mono>{row.listen_port}</Cell>
                  <Cell mono>{row.endpoint_host}</Cell>
                  <Cell mono><span className="inline-flex items-center gap-1"><span title={row.public_key}>{row.public_key.slice(0, 12)}…</span>
                    <CopyButton value={row.public_key} label={`Salin public key ${row.name}`} /></span></Cell>
                  <Cell>{row.enabled ? <Badge tone="online">Aktif</Badge> : <Badge tone="disabled">Nonaktif</Badge>}</Cell>
                  <Cell>{isAdmin ? <Button variant="ghost" size="sm" aria-label={`Ubah ${row.name}`}
                    onClick={() => setEditing(row)}><Pencil size={16} aria-hidden /> Ubah</Button> : null}</Cell>
                </tr>
              ))}
            </Table>
          )}
      </Card>
      {editing ? <InterfaceForm current={editing === 'new' ? null : editing} onClose={() => setEditing(null)} /> : null}
    </div>
  )
}

/** InterfaceForm: dialog tambah/ubah. Nama dikunci saat ubah (server menolak ganti nama). */
function InterfaceForm({ current, onClose }: { current: Interface | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>(current ? toForm(current) : EMPTY)
  const [error, setError] = useState<ApiError | null>(null)
  const [confirmSubnet, setConfirmSubnet] = useState(false)

  const save = useMutation({
    mutationFn: () => apiFetch<Interface>(current ? `/interfaces/${current.id}` : '/interfaces',
      { method: current ? 'PUT' : 'POST', ...jsonBody(toRequest(form)) }),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ['interfaces'] }); onClose() },
    onError: (caught) => setError(caught instanceof ApiError ? caught : new ApiError('unknown', caught.message, 0)),
  })

  const field = (key: keyof FormState) => ({
    value: form[key],
    error: error?.fieldMessage(key),
    onChange: (event: { target: { value: string } }) => setForm({ ...form, [key]: event.target.value }),
  })
  const subnetChanged = current !== null && form.address.trim() !== current.address

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }} title={current ? `Ubah ${current.name}` : 'Tambah interface'}
      description="Perubahan langsung diterapkan ke kernel; bila kernel menolak, tidak ada yang tersimpan.">
      <form className="grid gap-4" onSubmit={(event) => {
        event.preventDefault()
        setError(null)
        if (subnetChanged) setConfirmSubnet(true)
        else save.mutate()
      }}>
        <Field id="if-name" label="Nama" placeholder="wg0" disabled={current !== null} {...field('name')}
          hint={current ? 'Nama tidak bisa diganti.' : 'Huruf kecil/angka, maks 15, diawali huruf.'} />
        <Field id="if-address" label="Alamat server (CIDR)" placeholder="10.8.0.1/24" {...field('address')} />
        <div className="grid gap-4 sm:grid-cols-2">
          <Field id="if-port" label="Port UDP" inputMode="numeric" {...field('listen_port')} />
          <Field id="if-mtu" label="MTU" inputMode="numeric" hint="0 = bawaan (1420)" {...field('mtu')} />
        </div>
        <Field id="if-endpoint" label="Endpoint publik" placeholder="vpn.contoh.id" hint="Host atau IP, tanpa http:// dan port."
          {...field('endpoint_host')} />
        <Field id="if-dns" label="DNS client" hint="Dipisah koma." {...field('client_dns')} />
        <Field id="if-allowed" label="AllowedIPs client" hint="0.0.0.0/0 = semua lalu lintas lewat VPN." {...field('client_allowed_ips')} />
        <Field id="if-keepalive" label="PersistentKeepalive (detik)" inputMode="numeric" {...field('client_keepalive')} />
        {error && error.fields.length === 0 ? <p role="alert" className="text-sm text-destructive">{error.message}</p> : null}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose}>Batal</Button>
          <Button type="submit" disabled={save.isPending}>{save.isPending ? 'Menyimpan…' : 'Simpan'}</Button>
        </div>
      </form>
      {current ? (
        <ConfirmDialog open={confirmSubnet} onOpenChange={setConfirmSubnet} title="Ganti subnet?"
          description="Mengganti subnet mengubah alamat server di tunnel. Client lama perlu config baru."
          confirmLabel="Ganti subnet" confirmText={current.name}
          onConfirm={async () => { await save.mutateAsync() }} />
      ) : null}
    </Dialog>
  )
}
