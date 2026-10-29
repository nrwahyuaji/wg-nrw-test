// Badge status: online (hijau), offline (abu), disabled (kuning tua), neutral. Teks selalu ada, warna bukan satu-satunya
// penanda (aksesibel untuk buta warna).
import type { ReactNode } from 'react'
import { cn } from './cn'

type Tone = 'online' | 'offline' | 'disabled' | 'neutral' | 'danger'

const tones: Record<Tone, string> = {
  online: 'text-online border-online',
  offline: 'text-offline border-offline',
  disabled: 'text-disabled border-disabled',
  neutral: 'text-muted-foreground border-border',
  danger: 'text-destructive border-destructive',
}

/** Badge berbingkai dengan teks berwarna (kontras AA terhadap latar kartu). */
export function Badge({ tone = 'neutral', children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span className={cn('inline-flex items-center rounded-badge border px-2 py-0.5 text-xs font-medium', tones[tone])}>
      {children}
    </span>
  )
}
