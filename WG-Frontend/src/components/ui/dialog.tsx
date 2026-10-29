// Dialog modal di atas Radix Dialog (fokus terjebak di dalam, Esc menutup, judul diumumkan pembaca layar).
import { X } from 'lucide-react'
import type { ReactNode } from 'react'
import { Dialog as RadixDialog } from 'radix-ui'

/** Dialog terkontrol: `open` + `onOpenChange`. `description` wajib untuk aksesibilitas. */
export function Dialog({ open, onOpenChange, title, description, children }:
  { open: boolean; onOpenChange: (open: boolean) => void; title: string; description: string; children: ReactNode }) {
  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      <RadixDialog.Portal>
        <RadixDialog.Overlay className="fixed inset-0 bg-black/50" />
        <RadixDialog.Content className="fixed left-1/2 top-1/2 w-[min(32rem,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2
          rounded-card border border-border bg-card p-6 text-foreground shadow-xl max-h-[90vh] overflow-y-auto">
          {/* Tombol tutup eksplisit: di layar sentuh tidak ada tombol Esc. */}
          <RadixDialog.Close aria-label="Tutup" className="absolute right-3 top-3 inline-flex h-8 w-8 items-center justify-center
            rounded-control text-muted-foreground hover:bg-background"><X size={18} aria-hidden /></RadixDialog.Close>
          <RadixDialog.Title className="pr-8 text-lg font-semibold">{title}</RadixDialog.Title>
          <RadixDialog.Description className="mt-1 text-sm text-muted-foreground">{description}</RadixDialog.Description>
          <div className="mt-4">{children}</div>
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  )
}
