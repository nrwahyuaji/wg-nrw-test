// Kartu/panel dengan judul opsional.
import type { ReactNode } from 'react'
import { cn } from './cn'

/** Card: kontainer berbatas dengan radius kartu. */
export function Card({ title, actions, className, children }: { title?: string; actions?: ReactNode; className?: string; children: ReactNode }) {
  return (
    <section className={cn('min-w-0 rounded-card border border-border bg-card p-5', className)}>
      {title || actions ? (
        <header className="mb-4 flex items-center justify-between gap-3">
          {title ? <h2 className="text-lg font-semibold">{title}</h2> : <span />}
          {actions}
        </header>
      ) : null}
      {children}
    </section>
  )
}
