// Rute aplikasi (DESIGN 11). /login di luar Layout; rute lain di dalam Layout (butuh sesi: Layout memanggil /me,
// dan apiFetch mengalihkan ke /login saat 401). Halaman fase berikutnya masih penanda.
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createBrowserRouter, RouterProvider } from 'react-router'
import { Toaster } from 'sonner'
import { Layout } from './components/Layout'
import Account from './pages/Account'
import Dashboard from './pages/Dashboard'
import Interfaces from './pages/Interfaces'
import Login from './pages/Login'
import PeerDetail from './pages/PeerDetail'
import Peers from './pages/Peers'
import Placeholder from './pages/Placeholder'

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } } })

const router = createBrowserRouter([
  { path: '/login', element: <Login /> },
  {
    element: <Layout />,
    children: [
      { path: '/', element: <Dashboard /> },
      { path: '/interfaces', element: <Interfaces /> },
      { path: '/peers', element: <Peers /> },
      { path: '/peers/:id', element: <PeerDetail /> },
      { path: '/users', element: <Placeholder title="User" phase="Fase 13" /> },
      { path: '/audit', element: <Placeholder title="Audit" phase="Fase 10" /> },
      { path: '/settings', element: <Placeholder title="Pengaturan" phase="Fase 9" /> },
      { path: '/account', element: <Account /> },
      { path: '*', element: <Placeholder title="Halaman tidak ditemukan" phase="—" /> },
    ],
  },
])

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      <Toaster position="top-right" richColors closeButton />
    </QueryClientProvider>
  )
}
