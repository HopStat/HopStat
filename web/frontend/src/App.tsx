import { createBrowserRouter, RouterProvider, Navigate } from 'react-router-dom'
import { PublicLayout } from '@/components/layout/public-layout'
import { AdminLayout } from '@/components/layout/admin-layout'
import { QueryPage } from '@/pages/public/query-page'
import { LoginPage } from '@/pages/admin/login-page'
import { DashboardPage } from '@/pages/admin/dashboard-page'
import { NodesPage } from '@/pages/admin/nodes-page'
import { AuditPage } from '@/pages/admin/audit-page'
import { CommunityRulesPage } from '@/pages/admin/community-rules-page'
import { QuickQueriesPage } from '@/pages/admin/quick-queries-page'
import { BGPNeighborsPage } from '@/pages/admin/bgp-neighbors-page'
import { SettingsPage } from '@/pages/admin/settings-page'
import { GeoIPLookupPage } from '@/pages/admin/geoip-lookup-page'
import { NotFoundPage } from '@/pages/not-found-page'
import { useAuth } from '@/contexts/auth-context'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, ready } = useAuth()
  if (!ready) return null
  if (!isAuthenticated) return <Navigate to="/admin/login" replace />
  return <>{children}</>
}

const router = createBrowserRouter([
  {
    element: <PublicLayout />,
    children: [
      { path: '/', element: <QueryPage /> },
      { path: '/communities', element: <QueryPage /> },
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
      { path: 'audit', element: <AuditPage /> },
      { path: 'community-rules', element: <CommunityRulesPage /> },
      { path: 'quick-queries', element: <QuickQueriesPage /> },
      { path: 'bgp-neighbors', element: <BGPNeighborsPage /> },
      { path: 'geoip', element: <GeoIPLookupPage /> },
      { path: 'settings', element: <SettingsPage /> },
    ],
  },
  { path: '*', element: <NotFoundPage /> },
])

export default function App() {
  return <RouterProvider router={router} />
}
