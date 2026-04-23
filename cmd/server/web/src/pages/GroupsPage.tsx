import { useState, useEffect, useCallback } from 'react';
import { Card, Table, Button, Modal, Form, Input, Select, Tag, message, Space, Popconfirm, InputNumber, Row, Col } from 'antd';
import { PlusOutlined, DeleteOutlined, TeamOutlined, KeyOutlined } from '@ant-design/icons';
import { api } from '../services/api';
import type { Group, GroupMember, GroupCredential, GroupQuota } from '../types';

export default function GroupsPage() {
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm] = Form.useForm();

  // Detail modal state
  const [detailGroup, setDetailGroup] = useState<Group | null>(null);
  const [detailTab, setDetailTab] = useState<'members' | 'credentials' | 'quota'>('members');
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [credentials, setCredentials] = useState<GroupCredential[]>([]);
  const [groupQuota, setGroupQuota] = useState<GroupQuota | null>(null);
  const [addMemberOpen, setAddMemberOpen] = useState(false);
  const [addMemberForm] = Form.useForm();
  const [addCredOpen, setAddCredOpen] = useState(false);
  const [addCredForm] = Form.useForm();
  const [quotaForm] = Form.useForm();

  const fetchGroups = useCallback(() => {
    setLoading(true);
    api.listGroups().then((g) => setGroups(g ?? [])).catch(() => {}).finally(() => setLoading(false));
  }, []);

  useEffect(() => { fetchGroups(); }, [fetchGroups]);

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      await api.createGroup(values.name, values.description || '');
      message.success('Group created');
      setCreateOpen(false);
      createForm.resetFields();
      fetchGroups();
    } catch { message.error('Failed to create group'); }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.deleteGroup(id);
      message.success('Group deleted');
      fetchGroups();
      if (detailGroup?.id === id) setDetailGroup(null);
    } catch { message.error('Failed to delete group'); }
  };

  const openDetail = async (group: Group) => {
    setDetailGroup(group);
    setDetailTab('members');
    loadMembers(group.id);
  };

  const loadMembers = (groupId: string) => {
    api.listGroupMembers(groupId).then(setMembers).catch(() => {});
  };

  const loadCredentials = (groupId: string) => {
    api.listGroupCredentials(groupId).then(setCredentials).catch(() => {});
  };

  const loadQuota = (groupId: string) => {
    api.getGroupQuota(groupId).then(setGroupQuota).catch(() => {});
  };

  const handleTabChange = (tab: 'members' | 'credentials' | 'quota') => {
    setDetailTab(tab);
    if (!detailGroup) return;
    if (tab === 'members') loadMembers(detailGroup.id);
    if (tab === 'credentials') loadCredentials(detailGroup.id);
    if (tab === 'quota') loadQuota(detailGroup.id);
  };

  const handleAddMember = async () => {
    if (!detailGroup) return;
    try {
      const values = await addMemberForm.validateFields();
      await api.addGroupMember(detailGroup.id, values.user_id, values.role);
      message.success('Member added');
      setAddMemberOpen(false);
      addMemberForm.resetFields();
      loadMembers(detailGroup.id);
    } catch { message.error('Failed to add member'); }
  };

  const handleRemoveMember = async (userId: string) => {
    if (!detailGroup) return;
    try {
      await api.removeGroupMember(detailGroup.id, userId);
      message.success('Member removed');
      loadMembers(detailGroup.id);
    } catch { message.error('Failed to remove member'); }
  };

  const handleAddCredential = async () => {
    if (!detailGroup) return;
    try {
      const values = await addCredForm.validateFields();
      await api.setGroupCredential(detailGroup.id, values.provider, values.api_key, values.base_url);
      message.success('Credential set');
      setAddCredOpen(false);
      addCredForm.resetFields();
      loadCredentials(detailGroup.id);
    } catch { message.error('Failed to set credential'); }
  };

  const handleDeleteCredential = async (provider: string) => {
    if (!detailGroup) return;
    try {
      await api.deleteGroupCredential(detailGroup.id, provider);
      message.success('Credential deleted');
      loadCredentials(detailGroup.id);
    } catch { message.error('Failed to delete credential'); }
  };

  const handleSaveQuota = async () => {
    if (!detailGroup) return;
    try {
      const values = await quotaForm.validateFields();
      await api.setGroupQuota(detailGroup.id, values);
      message.success('Quota updated');
    } catch { message.error('Failed to update quota'); }
  };

  const memberColumns = [
    { title: 'Username', dataIndex: 'username', key: 'username' },
    { title: 'Role', dataIndex: 'role', key: 'role', render: (v: string) => <Tag color={v === 'admin' ? 'gold' : 'default'}>{v}</Tag> },
    { title: 'Joined', dataIndex: 'joined_at', key: 'joined_at', render: (v: string) => new Date(v).toLocaleDateString() },
    { title: 'Action', key: 'action', render: (_: unknown, r: GroupMember) => (
      <Popconfirm title="Remove member?" onConfirm={() => handleRemoveMember(r.user_id)}>
        <Button size="small" danger icon={<DeleteOutlined />} />
      </Popconfirm>
    )},
  ];

  const credColumns = [
    { title: 'Provider', dataIndex: 'provider', key: 'provider', render: (v: string) => <Tag color={v === 'openai' ? 'green' : v === 'anthropic' ? 'orange' : 'blue'}>{v}</Tag> },
    { title: 'Base URL', dataIndex: 'base_url', key: 'base_url', render: (v: string) => v || '-' },
    { title: 'Active', dataIndex: 'is_active', key: 'is_active', render: (v: boolean) => <Tag color={v ? 'green' : 'red'}>{v ? 'Yes' : 'No'}</Tag> },
    { title: 'Action', key: 'action', render: (_: unknown, r: GroupCredential) => (
      <Popconfirm title="Delete credential?" onConfirm={() => handleDeleteCredential(r.provider)}>
        <Button size="small" danger icon={<DeleteOutlined />} />
      </Popconfirm>
    )},
  ];

  return (
    <div style={{ display: 'flex', gap: 24 }}>
      <Card title="Groups" style={{ flex: 1 }} extra={<Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>New Group</Button>}>
        <Table<Group>
          dataSource={groups}
          columns={[
            { title: 'Name', dataIndex: 'name', key: 'name' },
            { title: 'Description', dataIndex: 'description', key: 'description', ellipsis: true },
            { title: 'Created', dataIndex: 'created_at', key: 'created_at', render: (v: string) => new Date(v).toLocaleDateString() },
            { title: 'Actions', key: 'actions', render: (_: unknown, r: Group) => (
              <Space>
                <Button size="small" onClick={() => openDetail(r)}>Manage</Button>
                <Popconfirm title="Delete this group?" onConfirm={() => handleDelete(r.id)}>
                  <Button size="small" danger>Delete</Button>
                </Popconfirm>
              </Space>
            )},
          ]}
          rowKey="id"
          loading={loading}
          pagination={false}
          size="small"
        />
      </Card>

      {detailGroup && (
        <Card
          title={detailGroup.name}
          style={{ width: 480 }}
          extra={<Button size="small" onClick={() => setDetailGroup(null)}>Close</Button>}
        >
          <Space style={{ marginBottom: 12 }}>
            <Button size="small" type={detailTab === 'members' ? 'primary' : 'default'} icon={<TeamOutlined />} onClick={() => handleTabChange('members')}>Members</Button>
            <Button size="small" type={detailTab === 'credentials' ? 'primary' : 'default'} icon={<KeyOutlined />} onClick={() => handleTabChange('credentials')}>Credentials</Button>
            <Button size="small" type={detailTab === 'quota' ? 'primary' : 'default'} onClick={() => handleTabChange('quota')}>Quota</Button>
          </Space>

          {detailTab === 'members' && (
            <>
              <Table<GroupMember> dataSource={members} columns={memberColumns} rowKey="user_id" pagination={false} size="small" />
              <Button size="small" icon={<PlusOutlined />} style={{ marginTop: 8 }} onClick={() => setAddMemberOpen(true)}>Add Member</Button>
            </>
          )}

          {detailTab === 'credentials' && (
            <>
              <Table<GroupCredential> dataSource={credentials} columns={credColumns} rowKey="provider" pagination={false} size="small" />
              <Button size="small" icon={<PlusOutlined />} style={{ marginTop: 8 }} onClick={() => setAddCredOpen(true)}>Add Credential</Button>
            </>
          )}

          {detailTab === 'quota' && groupQuota && (
            <Form form={quotaForm} layout="vertical" initialValues={groupQuota} onFinish={handleSaveQuota}>
              <Row gutter={8}>
                <Col span={12}><Form.Item name="rpm_limit" label="RPM Limit"><InputNumber style={{ width: '100%' }} min={0} /></Form.Item></Col>
                <Col span={12}><Form.Item name="rpd_limit" label="RPD Limit"><InputNumber style={{ width: '100%' }} min={0} /></Form.Item></Col>
                <Col span={12}><Form.Item name="token_daily" label="Token Daily"><InputNumber style={{ width: '100%' }} min={0} /></Form.Item></Col>
                <Col span={12}><Form.Item name="token_monthly" label="Token Monthly"><InputNumber style={{ width: '100%' }} min={0} /></Form.Item></Col>
                <Col span={12}><Form.Item name="spend_daily" label="Spend Daily (cents)"><InputNumber style={{ width: '100%' }} min={0} /></Form.Item></Col>
                <Col span={12}><Form.Item name="spend_monthly" label="Spend Monthly (cents)"><InputNumber style={{ width: '100%' }} min={0} /></Form.Item></Col>
              </Row>
              <Button type="primary" htmlType="submit" size="small">Save Quota</Button>
            </Form>
          )}
        </Card>
      )}

      <Modal title="Create Group" open={createOpen} onOk={handleCreate} onCancel={() => setCreateOpen(false)}>
        <Form form={createForm} layout="vertical">
          <Form.Item name="name" label="Name" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="description" label="Description"><Input.TextArea /></Form.Item>
        </Form>
      </Modal>

      <Modal title="Add Member" open={addMemberOpen} onOk={handleAddMember} onCancel={() => setAddMemberOpen(false)}>
        <Form form={addMemberForm} layout="vertical">
          <Form.Item name="user_id" label="User ID" rules={[{ required: true }]}><Input placeholder="Paste user UUID" /></Form.Item>
          <Form.Item name="role" label="Role" initialValue="member">
            <Select options={[{ value: 'admin', label: 'Admin' }, { value: 'member', label: 'Member' }]} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title="Add Credential" open={addCredOpen} onOk={handleAddCredential} onCancel={() => setAddCredOpen(false)}>
        <Form form={addCredForm} layout="vertical">
          <Form.Item name="provider" label="Provider" rules={[{ required: true }]}>
            <Select options={[{ value: 'openai', label: 'OpenAI' }, { value: 'anthropic', label: 'Anthropic' }, { value: 'gemini', label: 'Gemini' }]} />
          </Form.Item>
          <Form.Item name="api_key" label="API Key" rules={[{ required: true }]}><Input.Password /></Form.Item>
          <Form.Item name="base_url" label="Base URL (optional)"><Input placeholder="Leave empty for default" /></Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
