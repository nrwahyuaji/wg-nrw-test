// Konfirmasi aksi berbahaya (DESIGN 11): bila `confirmText` diisi, tombol baru aktif setelah pengguna mengetik nama
// target persis, sehingga hapus/ganti subnet tidak bisa terjadi karena salah klik.
import { useState } from 'react'
import { Button } from './button'
import { Dialog } from './dialog'
import { Field } from './input'

/** ConfirmDialog: `onConfirm` dipanggil lalu dialog ditutup bila berhasil; galat ditampilkan di dialog. */
export function ConfirmDialog({ open, onOpenChange, title, description, confirmLabel, confirmText, onConfirm }: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  confirmLabel: string
  confirmText?: string
  onConfirm: () => Promise<void>
}) {
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const ready = !confirmText || typed === confirmText

  // close mereset isian supaya dialog berikutnya tidak membawa ketikan lama.
  const close = (next: boolean) => {
    if (!next) {
      setTyped('')
      setError('')
    }
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={close} title={title} description={description}>
      <form
        className="grid gap-4"
        onSubmit={async (event) => {
          event.preventDefault()
          if (!ready || busy) return
          setBusy(true)
          setError('')
          try {
            await onConfirm()
            close(false)
          } catch (caught) {
            setError(caught instanceof Error ? caught.message : 'gagal')
          } finally {
            setBusy(false)
          }
        }}
      >
        {confirmText ? (
          <Field id="confirm-text" label={`Ketik "${confirmText}" untuk konfirmasi`} value={typed}
            autoComplete="off" onChange={(event) => setTyped(event.target.value)} />
        ) : null}
        {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => close(false)}>Batal</Button>
          <Button type="submit" variant="destructive" disabled={!ready || busy}>{confirmLabel}</Button>
        </div>
      </form>
    </Dialog>
  )
}
