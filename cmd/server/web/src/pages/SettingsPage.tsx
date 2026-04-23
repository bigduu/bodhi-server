import { useEffect, useState } from 'react';
import { Card, Switch, Table, Tag, Button, Popconfirm, message, Typography, Space, InputNumber, Row, Col, Modal } from 'antd';
import { SettingOutlined, PlusOutlined, DeleteOutlined } from '@ant-design/icons';
import { api } from '../services/api';
import type { InviteCode } from '../types';

const { Title } = Typography;

export default function SettingsPage() {
  const [settings, setSettings] = useState<Record<string, string>>({});
  const [invites, setInvites] = useState<InviteCode[]>([]);
  const [inviteModalOpen, setInviteModalOpen] = useState(false);

  useEffect(() => {
    api.listSettings().then(s => setSettings(s)).catch(() => {});
    api.listInvites().then(i => setInvites(i || [])).catch(() => {});
  }, []);

  const toggleSetting = async (key: string, value: boolean) => {
    try {
      await api.updateSetting(key, JSON.stringify(value));
      setSettings({ ...settings, [key]: JSON.stringify(value) });
      message.success('Setting updated');
    } catch { message.error('Failed to update setting'); }
  };

  const parseBool = (val: string | undefined, def: boolean): boolean => {
    if (!val) return def;
    try { return JSON.parse(val); } catch { return def; }
  };

  const createInvite = async (maxUses: number) => {
    try {
      await api.createInvite(maxUses);
      message.success('Invite code created');
      setInviteModalOpen(false);
      api.listInvites().then(i => setInvites(i || []));
    } catch { message.error('Failed to create invite'); }
  };

  const deleteInvite = async (code: string) => {
    try {
      await api.deleteInvite(code);
      message.success('Invite deleted');
      api.listInvites().then(i => setInvites(i || []));
    } catch { message.error('Failed to delete invite'); }
  };

  return (
    <div>
      <Title level={4} style={{ marginBottom: 24 }}>Settings</Title>

      <Card title={<><SettingOutlined /> Registration</>} style={{ marginBottom: 24 }}>
        <Row gutter={16}>
          <Col span={12}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <span>Allow Registration</span>
              <Switch checked={parseBool(settings['registration_enabled'], true)}
                onChange={(v) => toggleSetting('registration_enabled', v)} />
            </div>
          </Col>
          <Col span={12}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <span>Require Invite Code</span>
              <Switch checked={parseBool(settings['invite_required'], false)}
                onChange={(v) => toggleSetting('invite_required', v)} />
            </div>
          </Col>
        </Row>
      </Card>

      <Card title="Invite Codes" extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setInviteModalOpen(true)}>Create</Button>
      }>
        <Table
          dataSource={invites}
          rowKey="code"
          size="small"
          pagination={false}
          columns={[
            { title: 'Code', dataIndex: 'code', render: (v: string) => <Tag>{v}</Tag> },
            { title: 'Uses', render: (_: any, r: InviteCode) => `${r.used_count} / ${r.max_uses}` },
            { title: 'Expires', dataIndex: 'expires_at', render: (v: string | null) => v ? new Date(v).toLocaleString() : 'Never' },
            { title: 'Created', dataIndex: 'created_at', render: (v: string) => new Date(v).toLocaleString() },
            { title: '', width: 80, render: (_: any, r: InviteCode) => (
              <Popconfirm title="Delete this invite?" onConfirm={() => deleteInvite(r.code)}>
                <Button size="small" danger icon={<DeleteOutlined />} />
              </Popconfirm>
            )},
          ]}
        />
      </Card>

      <Modal title="Create Invite Code" open={inviteModalOpen} onCancel={() => setInviteModalOpen(false)}
        footer={null}>
        <InviteForm onSubmit={createInvite} />
      </Modal>
    </div>
  );
}

function InviteForm({ onSubmit }: { onSubmit: (maxUses: number) => void }) {
  const [maxUses, setMaxUses] = useState(1);
  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <div>
        <div style={{ marginBottom: 8 }}>Max Uses</div>
        <InputNumber min={0} value={maxUses} onChange={v => setMaxUses(v ?? 1)} style={{ width: '100%' }} />
        <div style={{ fontSize: 12, color: '#888', marginTop: 4 }}>0 = unlimited</div>
      </div>
      <Button type="primary" onClick={() => onSubmit(maxUses)} block>Create</Button>
    </Space>
  );
}
