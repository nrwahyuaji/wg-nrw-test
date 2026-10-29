// Input + label + pesan galat per field (dari 422 validation_failed). id wajib agar label terhubung (aksesibel).
import type { InputHTMLAttributes } from 'react'
import { cn } from './cn'

/** Field: label di atas, input, lalu galat (merah, diumumkan pembaca layar lewat aria-describedby). */
export function Field({ id, label, error, hint, className, ...props }:
  InputHTMLAttributes<HTMLInputElement> & { id: string; label: string; error?: string; hint?: string }) {
  const describedBy = error ? `${id}-error` : hint ? `${id}-hint` : undefined
  return (
    <div className="grid gap-1.5">
      <label htmlFor={id} className="text-sm font-medium">{label}</label>
      <input
        id={id}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        className={cn(
          'h-10 rounded-control border bg-card px-3 text-foreground',
          'focus-visible:outline-2 focus-visible:outline-primary',
          error ? 'border-destructive' : 'border-border',
          className,
        )}
        {...props}
      />
      {error ? <p id={`${id}-error`} className="text-sm text-destructive">{error}</p>
        : hint ? <p id={`${id}-hint`} className="text-sm text-muted-foreground">{hint}</p> : null}
    </div>
  )
}
