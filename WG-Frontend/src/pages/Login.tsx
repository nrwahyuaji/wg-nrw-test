// Halaman login (F-1). Pesan galat dari server ditampilkan apa adanya; 429 menyebut batas percobaan.
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { ApiError, apiFetch, jsonBody } from '../api/client'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/input'

export default function Login() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  return (
    <main className="grid min-h-screen place-items-center bg-background p-6 font-sans text-foreground">
      <form
        aria-labelledby="login-title"
        className="grid w-full max-w-sm gap-4 rounded-card border border-border bg-card p-8"
        onSubmit={async (event) => {
          event.preventDefault()
          setBusy(true)
          setError('')
          try {
            await apiFetch('/auth/login', { method: 'POST', ...jsonBody({ username, password }) })
            await queryClient.invalidateQueries({ queryKey: ['me'] })
            navigate('/', { replace: true })
          } catch (caught) {
            setError(caught instanceof ApiError ? caught.message : 'tidak bisa menghubungi server')
          } finally {
            setBusy(false)
          }
        }}
      >
        <h1 id="login-title" className="text-xl font-semibold">Masuk ke WG-WebUI</h1>
        <Field id="username" label="Username" autoComplete="username" required value={username}
          onChange={(event) => setUsername(event.target.value)} />
        <Field id="password" label="Password" type="password" autoComplete="current-password" required value={password}
          onChange={(event) => setPassword(event.target.value)} />
        {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
        <Button type="submit" disabled={busy}>{busy ? 'Memeriksa…' : 'Masuk'}</Button>
      </form>
    </main>
  )
}
