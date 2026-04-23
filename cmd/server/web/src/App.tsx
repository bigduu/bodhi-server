import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ConfigProvider } from 'antd';
import { AuthProvider, useAuth } from './hooks/useAuth';
import { ProtectedRoute } from './components/ProtectedRoute';
import AppLayout from './components/Layout';
import LoginPage from './pages/LoginPage';
import DashboardPage from './pages/DashboardPage';
import UsersPage from './pages/UsersPage';
import UserDetailPage from './pages/UserDetailPage';
import MetricsPage from './pages/MetricsPage';
import SettingsPage from './pages/SettingsPage';
import ModelsPage from './pages/ModelsPage';
import BillingPage from './pages/BillingPage';
import GroupsPage from './pages/GroupsPage';

function AppRoutes() {
  const { user, loading } = useAuth();
  if (loading) return null;

  return (
    <Routes>
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <LoginPage />} />
      <Route element={<ProtectedRoute><AppLayout /></ProtectedRoute>}>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/users" element={<UsersPage />} />
        <Route path="/users/:id" element={<UserDetailPage />} />
        <Route path="/metrics" element={<MetricsPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="/models" element={<ModelsPage />} />
        <Route path="/billing" element={<BillingPage />} />
        <Route path="/groups" element={<GroupsPage />} />
      </Route>
    </Routes>
  );
}

export default function App() {
  return (
    <ConfigProvider theme={{ token: { colorPrimary: '#0d9488' } }}>
      <BrowserRouter>
        <AuthProvider>
          <AppRoutes />
        </AuthProvider>
      </BrowserRouter>
    </ConfigProvider>
  );
}
