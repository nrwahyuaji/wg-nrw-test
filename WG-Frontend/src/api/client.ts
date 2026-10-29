// Klien API tunggal (PLAN 6.2, DESIGN 11): setiap request ke panel lewat apiFetch, yang menambahkan header CSRF,
// mengirim cookie sesi, mengurai envelope {data}/{error}, dan mengalihkan ke /login saat sesi berakhir (401).
import type { components } from './schema'

/** Skema bertipe hasil generate dari docs/openapi.yaml (`npm run api`). */
export type Schemas = components['schemas']
export type User = Schemas['User']
export type Interface = Schemas['Interface']
export type InterfaceRequest = Schemas['InterfaceRequest']
export type InterfaceHealth = Schemas['InterfaceHealth']
export type Peer = Schemas['Peer']
export type PeerRequest = Schemas['PeerRequest']
export type PeerStatus = Schemas['PeerStatus']
export type Dashboard = Schemas['Dashboard']
export type TrafficPoint = Schemas['TrafficPoint']
export type FieldError = Schemas['FieldError']

/** Galat dari API: code stabil (Inggris) untuk logika, message (Indonesia) untuk ditampilkan. */
export class ApiError extends Error {
  readonly code: string
  readonly status: number
  readonly fields: FieldError[]

  constructor(code: string, message: string, status: number, fields: FieldError[] = []) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.fields = fields
  }

  /** Pesan galat untuk satu field form (422 validation_failed), atau undefined. */
  fieldMessage(field: string): string | undefined {
    return this.fields.find((item) => item.field === field)?.message
  }
}

/** Hook navigasi saat 401; diganti di uji agar tidak menyentuh window.location. */
export const session = {
  redirectToLogin: () => window.location.assign('/login'),
}

/**
 * apiFetch memanggil `/api/v1{path}` dan mengembalikan isi `data` (undefined untuk 204).
 * 401 di luar halaman login → alihkan ke /login lalu lempar ApiError, supaya pemanggil tidak melanjutkan.
 * Body non-JSON (mis. HTML proxy) dilaporkan sebagai ApiError `bad_response`, bukan galat parse mentah.
 */
export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('X-Requested-With', 'wgpanel')
  if (init.body !== undefined && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(`/api/v1${path}`, { ...init, headers, credentials: 'same-origin' })

  if (response.status === 204) return undefined as T
  let payload: unknown
  try {
    payload = await response.json()
  } catch {
    throw new ApiError('bad_response', 'respons server tidak bisa dibaca', response.status)
  }
  if (response.ok) return (payload as { data: T }).data

  const body = (payload as { error?: Partial<Schemas['ApiErrorBody']> }).error ?? {}
  const error = new ApiError(body.code ?? 'unknown', body.message ?? `galat HTTP ${response.status}`, response.status, body.fields ?? [])
  if (response.status === 401 && !path.startsWith('/auth/')) session.redirectToLogin()
  throw error
}

/** jsonBody membungkus objek menjadi body request JSON. */
export function jsonBody(value: unknown): RequestInit {
  return { body: JSON.stringify(value) }
}
