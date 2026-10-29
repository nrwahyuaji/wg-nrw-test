// Tombol bersama. Varian mengikuti token desain: primary (aksi utama), outline, ghost, destructive (hapus/rotasi).
import type { ButtonHTMLAttributes } from 'react'
import { cn } from './cn'

type Variant = 'primary' | 'outline' | 'ghost' | 'destructive'

const variants: Record<Variant, string> = {
  primary: 'bg-primary text-primary-foreground hover:opacity-90',
  outline: 'border border-border bg-card text-foreground hover:bg-background',
  ghost: 'text-foreground hover:bg-background',
  destructive: 'bg-destructive text-primary-foreground hover:opacity-90',
}

/** Button: `variant` bawaan primary; `size="sm"` untuk aksi di baris tabel. */
export function Button({ variant = 'primary', size = 'md', className, type = 'button', ...props }:
  ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: 'sm' | 'md' }) {
  return (
    <button
      type={type}
      className={cn(
        'inline-flex items-center justify-center gap-2 rounded-control font-medium transition',
        'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary',
        'disabled:pointer-events-none disabled:opacity-50',
        size === 'sm' ? 'h-8 px-3 text-sm' : 'h-10 px-4',
        variants[variant],
        className,
      )}
      {...props}
    />
  )
}
