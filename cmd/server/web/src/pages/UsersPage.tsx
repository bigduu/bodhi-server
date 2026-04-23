import { useEffect, useState } from 'react';
import { Table, Tag, Button, Popconfirm, message, Input, Space, Typography } from 'antd';
import { SearchOutlined, DeleteOutlined, StopOutlined, CheckCircleOutlined, CrownOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { api } from '../services/api';
import type { UserListItem } from '../types';

const { Title } = Typography;

export default function UsersPage() {
  const [users, setUsers] = useState<UserListItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const navigate = useNavigate();

  const load = async (p: number) => {
    setLoading(true);
    try {
      const res = await api.listUsers(p);
      setUsers(res.items || []);
      setTotal(res.total);
      setPage(p);
    } catch { message.error('Failed to load users'); }
    finally { setLoading(false); }
  };

  useEffect(() => { load(1); }, []);

  const toggleActive = async (user: UserListItem) => {
    try {
      await api.updateUser(user.id, { is_active: !user.is_active });
      message.success(`User ${user.is_active ? 'disabled' : 'enabled'}`);
      load(page);
    } catch { message.error('Failed to update user'); }
  };

  const toggleAdmin = async (user: UserListItem) => {
    try {
      await api.updateUser(user.id, { is_admin: !user.is_admin });
      message.success(`Admin ${user.is_admin ? 'removed' : 'granted'}`);
      load(page);
    } catch { message.error('Failed to update user'); }
  };

  const deleteUser = async (id: string) => {
    try {
      await api.deleteUser(id);
      message.success('User deleted');
      load(page);
    } catch { message.error('Failed to delete user'); }
  };

  const filtered = search
    ? users.filter(u => u.username.toLowerCase().includes(search.toLowerCase()) || u.email.toLowerCase().includes(search.toLowerCase()))
    : users;

  const columns = [
    { title: 'Username', dataIndex: 'username', key: 'username', render: (u: string, r: UserListItem) => (
      <a onClick={() => navigate(`/users/${r.id}`)}>{u}</a>
    )},
    { title: 'Email', dataIndex: 'email', key: 'email' },
    { title: 'API Keys', dataIndex: 'api_key_count', key: 'keys', width: 80 },
    { title: 'Providers', dataIndex: 'cred_count', key: 'creds', width: 80 },
    { title: 'Status', key: 'status', width: 100, render: (_: any, r: UserListItem) => (
      <Space>
        {r.is_active ? <Tag color="green">Active</Tag> : <Tag color="red">Disabled</Tag>}
        {r.is_admin && <Tag color="gold"><CrownOutlined /> Admin</Tag>}
      </Space>
    )},
    { title: 'Last Login', dataIndex: 'last_login_at', key: 'last_login', width: 180, render: (v: string | null) => v ? new Date(v).toLocaleString() : '-' },
    { title: 'Created', dataIndex: 'created_at', key: 'created', width: 180, render: (v: string) => new Date(v).toLocaleString() },
    { title: 'Actions', key: 'actions', width: 200, render: (_: any, r: UserListItem) => (
      <Space>
        <Popconfirm title={`Sure to ${r.is_active ? 'disable' : 'enable'}?`} onConfirm={() => toggleActive(r)}>
          <Button size="small" icon={r.is_active ? <StopOutlined /> : <CheckCircleOutlined />} />
        </Popconfirm>
        <Popconfirm title={`Sure to ${r.is_admin ? 'remove admin' : 'grant admin'}?`} onConfirm={() => toggleAdmin(r)}>
          <Button size="small" icon={<CrownOutlined />} type={r.is_admin ? 'default' : 'primary'} />
        </Popconfirm>
        <Popconfirm title="Sure to delete this user?" onConfirm={() => deleteUser(r.id)}>
          <Button size="small" danger icon={<DeleteOutlined />} />
        </Popconfirm>
      </Space>
    )},
  ];

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}>Users</Title>
        <Input placeholder="Search users..." prefix={<SearchOutlined />} value={search} onChange={e => setSearch(e.target.value)} style={{ width: 250 }} />
      </div>
      <Table
        dataSource={filtered}
        columns={columns}
        rowKey="id"
        loading={loading}
        pagination={{ current: page, total, pageSize: 20, onChange: load }}
        size="middle"
      />
    </div>
  );
}
