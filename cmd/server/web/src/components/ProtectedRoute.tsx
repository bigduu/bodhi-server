import { Navigate } from 'react-router-dom';
import { useAuth } from '../hooks/useAuth';
import { Spin } from 'antd';

export function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <div style={{ display: 'flex', justifyContent: 'center', marginTop: 200 }}><Spin size="large" /></div>;
  if (!user) return <Navigate to="/login" replace />;
  return <>{children}</>;
}
