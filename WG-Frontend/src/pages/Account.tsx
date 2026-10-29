// Halaman akun: profil + ganti password (PUT /me/password). Sesi lain dicabut server setelah berhasil.
import { useState } from 'react'
import { ApiError, apiFetch, jsonBody } from '../api/client'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/input'
import { useMe } from '../hooks/useMe'

export default function Account() {
  const me = useMe()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [repeat, setRepeat] = useState('')
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null)

  return (
    <div className="grid max-w-xl grid-cols-[minmax(0,1fr)] gap-6">
      <h1 className="text-2xl font-semibold">Akun</h1>
      <Card title="Profil">
        <p><span className="text-muted-foreground">Username:</span> {me.data?.username}</p>
        <p><span className="text-muted-foreground">Role:</span> {me.data?.role}</p>
      </Card>
      <Card title="Ganti password">
        <form className="grid gap-4" onSubmit={async (event) => {
          event.preventDefault()
          if (next !== repeat) { setMessage({ ok: false, text: 'password baru dan ulangannya tidak sama' }); return }
          try {
            await apiFetch('/me/password', { method: 'PUT', ...jsonBody({ current_password: current, new_password: next }) })
            setCurrent(''); setNext(''); setRepeat('')
            setMessage({ ok: true, text: 'password diganti; sesi di perangkat lain sudah dikeluarkan' })
          } catch (caught) {
            setMessage({ ok: false, text: caught instanceof ApiError ? caught.message : 'gagal' })
          }
        }}>
          <Field id="current-password" label="Password lama" type="password" autoComplete="current-password" required
            value={current} onChange={(event) => setCurrent(event.target.value)} />
          <Field id="new-password" label="Password baru" type="password" autoComplete="new-password" required
            value={next} onChange={(event) => setNext(event.target.value)} />
          <Field id="repeat-password" label="Ulangi password baru" type="password" autoComplete="new-password" required
            value={repeat} onChange={(event) => setRepeat(event.target.value)} />
          {message ? <p role="status" className={message.ok ? 'text-sm text-online' : 'text-sm text-destructive'}>{message.text}</p> : null}
          <div><Button type="submit">Simpan password</Button></div>
        </form>
      </Card>
    </div>
  )
}
