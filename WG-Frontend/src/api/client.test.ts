// Uji apiFetch (PLAN 6.2): envelope sukses/galat, 204, header CSRF + cookie, 401 → redirect /login, 401 saat login
// tidak redirect, body bukan JSON → bad_response.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, apiFetch, jsonBody, session } from './client'

/** respond membuat Response palsu dengan status + body JSON (atau teks mentah). */
function respond(status: number, body?: unknown, raw?: string): Response {
  if (status === 204) return new Response(null, { status })
  return new Response(raw ?? JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('apiFetch', () => {
  const fetchMock = vi.fn<typeof fetch>()
  let redirect: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.stubGlobal('fetch', fetchMock)
    redirect = vi.fn()
    session.redirectToLogin = redirect as () => void
  })
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('mengurai {data} dan mengirim header CSRF + cookie', async () => {
    fetchMock.mockResolvedValue(respond(200, { data: { id: 1, username: 'admin', role: 'admin' } }))
    const user = await apiFetch<{ username: string }>('/me')
    expect(user.username).toBe('admin')
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/me')
    expect(new Headers(init?.headers).get('X-Requested-With')).toBe('wgpanel')
    expect(init?.credentials).toBe('same-origin')
  })

  it('menambahkan Content-Type JSON bila ada body', async () => {
    fetchMock.mockResolvedValue(respond(201, { data: {} }))
    await apiFetch('/peers', { method: 'POST', ...jsonBody({ name: 'x' }) })
    const [, init] = fetchMock.mock.calls[0]
    expect(new Headers(init?.headers).get('Content-Type')).toBe('application/json')
    expect(init?.body).toBe('{"name":"x"}')
  })

  it('204 → undefined', async () => {
    fetchMock.mockResolvedValue(respond(204))
    await expect(apiFetch('/auth/logout', { method: 'POST' })).resolves.toBeUndefined()
  })

  it('galat → ApiError dengan code, message, status, fields', async () => {
    fetchMock.mockResolvedValue(respond(422, {
      error: { code: 'validation_failed', message: 'periksa kembali isian', fields: [{ field: 'name', message: 'wajib' }] },
    }))
    const error = await apiFetch('/interfaces', { method: 'POST' }).catch((caught: unknown) => caught)
    expect(error).toBeInstanceOf(ApiError)
    const apiError = error as ApiError
    expect([apiError.code, apiError.message, apiError.status]).toEqual(['validation_failed', 'periksa kembali isian', 422])
    expect(apiError.fieldMessage('name')).toBe('wajib')
    expect(apiError.fieldMessage('address')).toBeUndefined()
    expect(redirect).not.toHaveBeenCalled()
  })

  it('401 → redirect ke /login lalu melempar', async () => {
    fetchMock.mockResolvedValue(respond(401, { error: { code: 'unauthenticated', message: 'silakan login' } }))
    await expect(apiFetch('/me')).rejects.toMatchObject({ code: 'unauthenticated', status: 401 })
    expect(redirect).toHaveBeenCalledOnce()
  })

  it('401 dari /auth/login tidak redirect (pesan ditampilkan di form)', async () => {
    fetchMock.mockResolvedValue(respond(401, { error: { code: 'invalid_credentials', message: 'username atau password salah' } }))
    await expect(apiFetch('/auth/login', { method: 'POST' })).rejects.toMatchObject({ code: 'invalid_credentials' })
    expect(redirect).not.toHaveBeenCalled()
  })

  it('body bukan JSON → bad_response', async () => {
    fetchMock.mockResolvedValue(respond(502, undefined, '<html>Bad Gateway</html>'))
    await expect(apiFetch('/me')).rejects.toMatchObject({ code: 'bad_response', status: 502 })
  })
})
