import { useState, useEffect } from 'react';
import { Card, Form, Input, Button, Typography, message } from 'antd';
import { UserOutlined, LockOutlined, MailOutlined, GiftOutlined } from '@ant-design/icons';
import { useAuth } from '../hooks/useAuth';
import { useNavigate } from 'react-router-dom';
import { api } from '../services/api';

const { Title } = Typography;

export default function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const [isRegister, setIsRegister] = useState(false);
  const [regEnabled, setRegEnabled] = useState(true);

  useEffect(() => {
    // Check registration status
    api.listSettings().then(s => {
      try { setRegEnabled(JSON.parse(s['registration_enabled'] ?? 'true')); } catch {}
    }).catch(() => {});
  }, []);

  const onFinish = async (values: { username: string; password: string; email?: string; invite_code?: string }) => {
    setLoading(true);
    try {
      if (isRegister) {
        await api.register(values.username, values.password, values.email, values.invite_code);
      }
      await login(values.username, values.password);
      message.success('Welcome!');
      navigate('/');
    } catch (e: any) {
      message.error(e.message || (isRegister ? 'Registration failed' : 'Login failed'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: '#f5f5f5' }}>
      <Card style={{ width: 400 }}>
        <Title level={3} style={{ textAlign: 'center', marginBottom: 32 }}>Bodhi Admin</Title>
        <Form onFinish={onFinish} size="large">
          <Form.Item name="username" rules={[{ required: true, min: 3, message: 'Username >= 3 chars' }]}>
            <Input prefix={<UserOutlined />} placeholder="Username" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, min: 8, message: 'Password >= 8 chars' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="Password" />
          </Form.Item>
          {isRegister && (
            <>
              <Form.Item name="email">
                <Input prefix={<MailOutlined />} placeholder="Email (optional)" />
              </Form.Item>
              <Form.Item name="invite_code">
                <Input prefix={<GiftOutlined />} placeholder="Invite Code (if required)" />
              </Form.Item>
            </>
          )}
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block>
              {isRegister ? 'Register' : 'Login'}
            </Button>
          </Form.Item>
        </Form>
        {regEnabled && (
          <div style={{ textAlign: 'center' }}>
            <Typography.Text type="secondary">
              {isRegister ? 'Already have an account? ' : "Don't have an account? "}
            </Typography.Text>
            <Typography.Link onClick={() => setIsRegister(!isRegister)}>
              {isRegister ? 'Login' : 'Register'}
            </Typography.Link>
          </div>
        )}
      </Card>
    </div>
  );
}
