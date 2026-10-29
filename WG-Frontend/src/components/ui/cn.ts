// cn menggabungkan nama class Tailwind, membuang nilai kosong/false (tanpa dependensi tambahan).
export function cn(...classes: (string | false | null | undefined)[]): string {
  return classes.filter(Boolean).join(' ')
}
