// useMe: profil user login (GET /me). Dipakai Layout untuk menyaring menu per role dan menampilkan nama.
import { useQuery } from '@tanstack/react-query'
import { apiFetch, type User } from '../api/client'

export function useMe() {
  return useQuery({ queryKey: ['me'], queryFn: () => apiFetch<User>('/me'), staleTime: 60_000, retry: false })
}
