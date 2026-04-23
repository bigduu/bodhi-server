import { Layout as AntLayout, Menu, Button, Typography, theme } from 'antd';
import { DashboardOutlined, UserOutlined, BarChartOutlined, SettingOutlined, AppstoreOutlined, DollarOutlined, TeamOutlined, LogoutOutlined } from '@ant-design/icons';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../hooks/useAuth';

const { Header, Sider, Content } = AntLayout;
const { Text } = Typography;

export default function AppLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout } = useAuth();
  const { token } = theme.useToken();

  const menuItems = [
    { key: '/', icon: <DashboardOutlined />, label: 'Dashboard' },
    { key: '/users', icon: <UserOutlined />, label: 'Users' },
    { key: '/groups', icon: <TeamOutlined />, label: 'Groups' },
    { key: '/models', icon: <AppstoreOutlined />, label: 'Models' },
    { key: '/billing', icon: <DollarOutlined />, label: 'Billing' },
    { key: '/metrics', icon: <BarChartOutlined />, label: 'Metrics' },
    { key: '/settings', icon: <SettingOutlined />, label: 'Settings' },
  ];

  const selectedKey = location.pathname.startsWith('/users') ? '/users'
    : location.pathname.startsWith('/groups') ? '/groups'
    : location.pathname.startsWith('/models') ? '/models'
    : location.pathname.startsWith('/billing') ? '/billing'
    : location.pathname.startsWith('/metrics') ? '/metrics'
    : location.pathname.startsWith('/settings') ? '/settings'
    : '/';

  return (
    <AntLayout style={{ minHeight: '100vh' }}>
      <Header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0 24px', background: token.colorBgContainer }}>
        <Text strong style={{ fontSize: 18 }}>Bodhi Admin</Text>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <Text type="secondary">{user?.username}</Text>
          <Button type="text" icon={<LogoutOutlined />} onClick={() => { logout(); navigate('/login'); }}>Logout</Button>
        </div>
      </Header>
      <AntLayout>
        <Sider width={200} style={{ background: token.colorBgContainer }}>
          <Menu mode="inline" selectedKeys={[selectedKey]} items={menuItems} onClick={({ key }) => navigate(key)} style={{ marginTop: 16 }} />
        </Sider>
        <Content style={{ margin: 24, padding: 24, background: token.colorBgContainer, borderRadius: token.borderRadiusLG, minHeight: 280 }}>
          <Outlet />
        </Content>
      </AntLayout>
    </AntLayout>
  );
}
