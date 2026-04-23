import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Card, Descriptions, Table, Tag, Button, Popconfirm, message, Space, Row, Col, Modal, Form, InputNumber, Select, Progress } from 'antd';
import { ArrowLeftOutlined, DeleteOutlined, KeyOutlined, CloudServerOutlined, BarChartOutlined, SettingOutlined } from '@ant-design/icons';
import { api } from '../services/api';
import type { UserListItem, APIKey, Credential, UsageDetail, QuotaData } from '../types';

export default function UserDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [user, setUser] = useState<UserListItem | null>(null);
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [creds, setCreds] = useState<Credential[]>([]);
  const [usage, setUsage] = useState<UsageDetail[]>([]);
  const [usageTotal, setUsageTotal] = useState(0);
  const [usagePage, setUsagePage] = useState(1);
  const [quotaData, setQuotaData] = useState<QuotaData | null>(null);
  const [quotaModalOpen, setQuotaModalOpen] = useState(false);
  const [quotaForm] = Form.useForm();

  useEffect(() => {
    if (!id) return;
    api.listUsers(1, 1000).then(res => {
      const u = (res.items || []).find(u => u.id === id);
      if (u) setUser(u);
    });
    api.listUserKeys(id).then(setKeys).catch(() => {});
    api.listUserCredentials(id).then(setCreds).catch(() => {});
    api.userUsage(id, 1).then(res => { setUsage(res.items || []); setUsageTotal(res.total); }).catch(() => {});
    api.getQuota(id).then(setQuotaData).catch(() => {});
  }, [id]);

  const revokeKey = async (keyId: string) => {
    if (!id) return;
    try {
      await api.deleteUserKey(id, keyId);
      message.success('Key revoked');
      api.listUserKeys(id).then(setKeys);
    } catch { message.error('Failed to revoke key'); }
  };

  const deleteCred = async (provider: string) => {
    if (!id) return;
    try {
      await api.deleteUserCredential(id, provider);
      message.success('Credential deleted');
      api.listUserCredentials(id).then(setCreds);
    } catch { message.error('Failed to delete credential'); }
  };

  const loadUsage = async (p: number) => {
    if (!id) return;
    try {
      const res = await api.userUsage(id, p);
      setUsage(res.items || []);
      setUsageTotal(res.total);
      setUsagePage(p);
    } catch {}
  };

  const openQuotaModal = () => {
    if (quotaData) {
      quotaForm.setFieldsValue(quotaData.quota);
    }
    setQuotaModalOpen(true);
  };

  const saveQuota = async () => {
    if (!id) return;
    try {
      const values = await quotaForm.validateFields();
      await api.setQuota(id, values);
      message.success('Quota updated');
      setQuotaModalOpen(false);
      api.getQuota(id).then(setQuotaData);
    } catch { message.error('Failed to update quota'); }
  };

  const pct = (used: number, limit: number) => {
    if (limit <= 0) return -1;
    return Math.min(100, Math.round((used / limit) * 100));
  };

  if (!user) return null;

  const q = quotaData?.quota;
  const c = quotaData?.counter;

  return (
    <div>
      <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/users')} style={{ marginBottom: 16 }}>Back to Users</Button>

      <Card style={{ marginBottom: 16 }}>
        <Descriptions title={user.username} extra={
          <Space>
            {user.is_active ? <Tag color="green">Active</Tag> : <Tag color="red">Disabled</Tag>}
            {user.is_admin && <Tag color="gold">Admin</Tag>}
            <Button icon={<SettingOutlined />} onClick={openQuotaModal}>Edit Quota</Button>
          </Space>
        }>
          <Descriptions.Item label="ID">{user.id}</Descriptions.Item>
          <Descriptions.Item label="Email">{user.email || '-'}</Descriptions.Item>
          <Descriptions.Item label="Created">{new Date(user.created_at).toLocaleString()}</Descriptions.Item>
          <Descriptions.Item label="Last Login">{user.last_login_at ? new Date(user.last_login_at).toLocaleString() : '-'}</Descriptions.Item>
        </Descriptions>
      </Card>

      {/* Quota Progress */}
      {q && c && (
        <Card title="Quota Usage" size="small" style={{ marginBottom: 16 }}>
          <Row gutter={16}>
            <Col span={4}>
              <Progress type="dashboard" percent={pct(c.MinuteRequests, q.RPM)} format={() => `${c.MinuteRequests}/${q.RPM || '∞'}`} />
              <div style={{ textAlign: 'center', fontSize: 12, marginTop: 4 }}>RPM</div>
            </Col>
            <Col span={4}>
              <Progress type="dashboard" percent={pct(c.DayRequests, q.RPD)} format={() => `${c.DayRequests}/${q.RPD || '∞'}`} />
              <div style={{ textAlign: 'center', fontSize: 12, marginTop: 4 }}>RPD</div>
            </Col>
            <Col span={4}>
              <Progress type="dashboard" percent={pct(Number(c.DayTokens), Number(q.TokenDaily))} format={() => `${(Number(c.DayTokens) / 1000).toFixed(1)}k/${q.TokenDaily ? (q.TokenDaily / 1000) + 'k' : '∞'}`} />
              <div style={{ textAlign: 'center', fontSize: 12, marginTop: 4 }}>Daily Tokens</div>
            </Col>
            <Col span={4}>
              <Progress type="dashboard" percent={pct(Number(c.MonthTokens), Number(q.TokenMonthly))} format={() => `${(Number(c.MonthTokens) / 1000).toFixed(1)}k/${q.TokenMonthly ? (q.TokenMonthly / 1000) + 'k' : '∞'}`} />
              <div style={{ textAlign: 'center', fontSize: 12, marginTop: 4 }}>Monthly Tokens</div>
            </Col>
            <Col span={4}>
              <Progress type="dashboard" percent={pct(c.DaySpend, q.SpendDaily)} format={() => `$${(c.DaySpend / 100).toFixed(2)}/$${q.SpendDaily ? (q.SpendDaily / 100).toFixed(0) : '∞'}`} />
              <div style={{ textAlign: 'center', fontSize: 12, marginTop: 4 }}>Daily Spend</div>
            </Col>
            <Col span={4}>
              <Progress type="dashboard" percent={pct(c.MonthSpend, q.SpendMonthly)} format={() => `$${(c.MonthSpend / 100).toFixed(2)}/$${q.SpendMonthly ? (q.SpendMonthly / 100).toFixed(0) : '∞'}`} />
              <div style={{ textAlign: 'center', fontSize: 12, marginTop: 4 }}>Monthly Spend</div>
            </Col>
          </Row>
          {q.AllowedModels && q.AllowedModels.length > 0 && (
            <div style={{ marginTop: 12 }}>
              <span style={{ fontSize: 12, color: '#888' }}>Allowed Models: </span>
              {q.AllowedModels.map(m => <Tag key={m}>{m}</Tag>)}
            </div>
          )}
        </Card>
      )}

      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={12}>
          <Card title={<><KeyOutlined /> API Keys ({keys.length})</>} size="small">
            <Table
              dataSource={keys}
              rowKey="id"
              size="small"
              pagination={false}
              columns={[
                { title: 'Name', dataIndex: 'name' },
                { title: 'Prefix', dataIndex: 'key_prefix', render: (v: string) => <Tag>{v}...</Tag> },
                { title: 'Status', dataIndex: 'is_active', render: (v: boolean) => v ? <Tag color="green">Active</Tag> : <Tag color="red">Revoked</Tag> },
                { title: 'Last Used', dataIndex: 'last_used_at', render: (v: string | null) => v ? new Date(v).toLocaleString() : 'Never' },
                { title: '', width: 80, render: (_: any, r: APIKey) => r.is_active ? (
                  <Popconfirm title="Revoke this key?" onConfirm={() => revokeKey(r.id)}>
                    <Button size="small" danger icon={<DeleteOutlined />}>Revoke</Button>
                  </Popconfirm>
                ) : null },
              ]}
            />
          </Card>
        </Col>
        <Col span={12}>
          <Card title={<><CloudServerOutlined /> Provider Credentials ({creds.length})</>} size="small">
            <Table
              dataSource={creds}
              rowKey="provider"
              size="small"
              pagination={false}
              columns={[
                { title: 'Provider', dataIndex: 'provider', render: (v: string) => <Tag color="blue">{v}</Tag> },
                { title: 'Base URL', dataIndex: 'base_url', render: (v: string) => v || 'Default' },
                { title: '', width: 80, render: (_: any, r: Credential) => (
                  <Popconfirm title="Delete this credential?" onConfirm={() => deleteCred(r.provider)}>
                    <Button size="small" danger icon={<DeleteOutlined />} />
                  </Popconfirm>
                ) },
              ]}
            />
          </Card>
        </Col>
      </Row>

      <Card title={<><BarChartOutlined /> Usage History</>}>
        <Table
          dataSource={usage}
          rowKey="id"
          size="small"
          pagination={{ current: usagePage, total: usageTotal, pageSize: 20, onChange: loadUsage }}
          columns={[
            { title: 'Time', dataIndex: 'created_at', width: 180, render: (v: string) => new Date(v).toLocaleString() },
            { title: 'Provider', dataIndex: 'provider', render: (v: string) => <Tag>{v}</Tag> },
            { title: 'Model', dataIndex: 'model' },
            { title: 'Tokens', dataIndex: 'tokens', width: 100 },
            { title: 'Duration (ms)', dataIndex: 'duration_ms', width: 120 },
            { title: 'Status', dataIndex: 'status_code', width: 80, render: (v: number) => v < 400 ? <Tag color="green">{v}</Tag> : <Tag color="red">{v}</Tag> },
            { title: 'Error', dataIndex: 'error_message', ellipsis: true },
          ]}
        />
      </Card>

      {/* Quota Edit Modal */}
      <Modal title="Edit User Quota" open={quotaModalOpen} onOk={saveQuota} onCancel={() => setQuotaModalOpen(false)} width={600}>
        <Form form={quotaForm} layout="vertical">
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="RPM" label="Requests Per Minute (0=unlimited)">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="RPD" label="Requests Per Day (0=unlimited)">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="TokenDaily" label="Daily Token Limit (0=unlimited)">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="TokenMonthly" label="Monthly Token Limit (0=unlimited)">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="SpendDaily" label="Daily Spend (cents, 0=unlimited)">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="SpendMonthly" label="Monthly Spend (cents, 0=unlimited)">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="AllowedModels" label="Allowed Models (empty=all)">
            <Select mode="tags" placeholder="e.g. gpt-4o, claude-sonnet-4*" style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
