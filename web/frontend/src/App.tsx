import { createBrowserRouter, RouterProvider, Navigate } from 'react-router-dom'
import { PublicLayout } from '@/components/layout/public-layout'
import { AdminLayout } from '@/components/layout/admin-layout'
import { QueryPage } from '@/pages/public/query-page'
import { LoginPage } from '@/pages/admin/login-page'
import { DashboardPage } from '@/pages/admin/dashboard-page'
import { NodesPage } from '@/pages/admin/nodes-page'
import { UsersPage } from '@/pages/admin/users-page'
import { AuditPage } from '@/pages/admin/audit-page'
import { CommunityRulesPage } from '@/pages/admin/community-rules-page'
import { BGPNeighborsPage } from '@/pages/admin/bgp-neighbors-page'
import { SettingsPage } from '@/pages/admin/settings-page'
import { NotFoundPage } from '@/pages/not-found-page'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const token = localStorage.getItem('jwt_token')
  if (!token) return <Navigate to="/admin/login" replace />
  return <>{children}</>
}

const router = createBrowserRouter([
  {
    element: <PublicLayout />,
    children: [
      { path: '/', element: <QueryPage /> },
    ],
  },
  {
    path: '/admin/login',
    element: <LoginPage />,
  },
  {
    path: '/admin',
    element: <RequireAuth><AdminLayout /></RequireAuth>,
    children: [
      { index: true, element: <DashboardPage /> },
      { path: 'nodes', element: <NodesPage /> },
      { path: 'users', element: <UsersPage /> },
      { path: 'audit', element: <AuditPage /> },
      { path: 'community-rules', element: <CommunityRulesPage /> },
      { path: 'bgp-neighbors', element: <BGPNeighborsPage /> },
      { path: 'settings', element: <SettingsPage /> },
    ],
  },
  { path: '*', element: <NotFoundPage /> },
])

export default function App() {
  return <RouterProvider router={router} />
}
