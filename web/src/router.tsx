import { createBrowserRouter, Navigate } from 'react-router-dom'
import AdminLayout from './components/AdminLayout'
import RequireBootstrap from './components/BootstrapGate'
import LoginPage from './pages/login'
import SetupPage from './pages/setup'
import DashboardPage from './pages/dashboard'
import ReposPage from './pages/repos'
import RepoDetailPage from './pages/repos/detail'
import CredentialsPage from './pages/credentials'
import ReviewsPage from './pages/reviews'
import ReviewDetailPage from './pages/reviews/detail'
import JobsPage from './pages/jobs'
import AuthorsStatsPage from './pages/stats/authors'
import MembersPage from './pages/members'
import SettingsPage from './pages/settings'
import PublicReportPage from './pages/public/report'
import PublicAuthorReportPage from './pages/public/authorReport'

export const router = createBrowserRouter([
  { path: '/login', element: <RequireBootstrap><LoginPage /></RequireBootstrap> },
  { path: '/setup', element: <SetupPage /> },
  { path: '/reports/:token', element: <PublicReportPage /> },
  { path: '/author-reports/:token', element: <PublicAuthorReportPage /> },
  {
    path: '/admin',
    element: <RequireBootstrap><AdminLayout /></RequireBootstrap>,
    children: [
      { index: true, element: <Navigate to="/admin/dashboard" replace /> },
      { path: 'dashboard', element: <DashboardPage /> },
      { path: 'repos', element: <ReposPage /> },
      { path: 'repos/:id', element: <RepoDetailPage /> },
      { path: 'credentials', element: <CredentialsPage /> },
      { path: 'reviews', element: <ReviewsPage /> },
      { path: 'reviews/:id', element: <ReviewDetailPage /> },
      { path: 'stats/authors', element: <AuthorsStatsPage /> },
      { path: 'members', element: <MembersPage /> },
      { path: 'jobs', element: <JobsPage /> },
      { path: 'settings', element: <SettingsPage /> },
    ],
  },
  { path: '/', element: <Navigate to="/admin" replace /> },
  { path: '*', element: <Navigate to="/admin" replace /> },
])
