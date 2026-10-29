// Tombol salin nilai panjang (public key) yang di tabel ditampilkan terpotong (temuan T6-04).
import { Check, Copy } from 'lucide-react'
import { useState } from 'react'

/** CopyButton: menyalin `value` ke clipboard; ikon berubah jadi centang 1,5 detik dan "Tersalin" diumumkan pembaca
 *  layar. Area sentuh 44 px di HP (WCAG 2.5.5), 32 px di desktop. `relative` wajib: tanpa itu teks sr-only
 *  (position absolute) berjangkar di luar wadah gulir tabel dan melebarkan dokumen di HP (terukur 548 px pada 360 px). */
export function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button type="button" aria-label={label} title={label}
      className="relative inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-control text-muted-foreground hover:bg-background md:h-8 md:w-8"
      onClick={async () => {
        await navigator.clipboard?.writeText(value)
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
      }}>
      {copied ? <Check size={16} aria-hidden /> : <Copy size={16} aria-hidden />}
      <span className="sr-only" aria-live="polite">{copied ? 'Tersalin' : ''}</span>
    </button>
  )
}
