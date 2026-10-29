// Tabel sederhana yang bisa digulir horizontal di layar sempit.
import type { ReactNode } from 'react'

/** Table: kepala tabel dari `columns`, isi dari children (baris <tr>). `empty` tampil bila tidak ada baris. */
export function Table({ columns, children, empty, caption }: { columns: string[]; children: ReactNode; empty?: ReactNode; caption: string }) {
  return (
    // Bayangan di tepi kanan menandakan tabel bisa digeser (T6-04). Lapisan penutup (warna kartu) ikut tergulir
    // ("local") sehingga bayangan HANYA tampak bila masih ada isi di kanan; tabel yang muat tidak berbayang.
    <div className="overflow-x-auto [background:linear-gradient(to_left,var(--color-card)_30%,transparent)_right/2.5rem_100%_no-repeat_local,radial-gradient(farthest-side_at_100%_50%,rgb(0_0_0/0.12),transparent)_right/0.625rem_100%_no-repeat_scroll]">
      <table className="w-full text-left text-sm">
        <caption className="sr-only">{caption}</caption>
        <thead className="border-b border-border text-muted-foreground">
          <tr>{columns.map((column) => <th key={column} scope="col" className="px-3 py-2 font-medium">{column}</th>)}</tr>
        </thead>
        <tbody className="divide-y divide-border">{children}</tbody>
      </table>
      {empty}
    </div>
  )
}

/** Cell: sel isi tabel. `mono` untuk IP/kunci/CIDR (font mono + tabular-nums sesuai token desain). */
export function Cell({ children, mono }: { children: ReactNode; mono?: boolean }) {
  return <td className={mono ? 'px-3 py-2 font-mono tabular-nums' : 'px-3 py-2'}>{children}</td>
}
