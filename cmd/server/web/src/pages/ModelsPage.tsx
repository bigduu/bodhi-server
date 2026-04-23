import { useEffect, useState } from 'react';
import { Tabs, Table, Tag, Button, Popconfirm, message, Typography, Modal, Form, Input, InputNumber, Select, Switch, Row, Col } from 'antd';
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons';
import { api } from '../services/api';
import type { ModelEntry } from '../types';

const { Title } = Typography;

interface ProviderInstance {
  id: string;
  model_id: string;
  provider_type: string;
  instance_name: string;
  priority: number;
  base_url: string;
  is_active: boolean;
  health_status: string;
}

export default function ModelsPage() {
  const [models, setModels] = useState<ModelEntry[]>([]);
  const [instances, setInstances] = useState<ProviderInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const [modelModalOpen, setModelModalOpen] = useState(false);
  const [instanceModalOpen, setInstanceModalOpen] = useState(false);
  const [editingModel, setEditingModel] = useState<ModelEntry | null>(null);
  const [editingInstance, setEditingInstance] = useState<ProviderInstance | null>(null);
  const [modelForm] = Form.useForm();
  const [instanceForm] = Form.useForm();

  const load = async () => {
    setLoading(true);
    try {
      const [ml, il] = await Promise.all([api.listModels(), api.listInstances()]);
      setModels(ml || []);
      setInstances(il || []);
    } catch { message.error('Failed to load'); }
    finally { setLoading(false); }
  };

  useEffect(() => { load(); }, []);

  // Model CRUD
  const openCreateModel = () => {
    setEditingModel(null);
    modelForm.resetFields();
    modelForm.setFieldsValue({ provider: 'openai', is_active: true, is_featured: false, sort_order: 0, context_window: 0, max_output: 0 });
    setModelModalOpen(true);
  };

  const openEditModel = (m: ModelEntry) => {
    setEditingModel(m);
    modelForm.setFieldsValue(m);
    setModelModalOpen(true);
  };

  const saveModel = async () => {
    try {
      const values = await modelForm.validateFields();
      if (values.capabilities && typeof values.capabilities === 'string') {
        values.capabilities = (values.capabilities as string).split(',').map((s: string) => s.trim()).filter(Boolean);
      }
      if (editingModel) { await api.updateModel(editingModel.id, values); }
      else { await api.createModel(values); }
      message.success('Model saved');
      setModelModalOpen(false);
      load();
    } catch { message.error('Failed to save model'); }
  };

  const toggleModel = async (m: ModelEntry) => {
    try { await api.updateModel(m.id, { is_active: !m.is_active }); load(); }
    catch { message.error('Failed to toggle'); }
  };

  // Instance CRUD
  const openCreateInstance = () => {
    setEditingInstance(null);
    instanceForm.resetFields();
    instanceForm.setFieldsValue({ provider_type: 'openai', priority: 0, is_active: true });
    setInstanceModalOpen(true);
  };

  const openEditInstance = (inst: ProviderInstance) => {
    setEditingInstance(inst);
    instanceForm.setFieldsValue(inst);
    setInstanceModalOpen(true);
  };

  const saveInstance = async () => {
    try {
      const values = await instanceForm.validateFields();
      if (editingInstance) { await api.updateInstance(editingInstance.id, values); }
      else { await api.createInstance(values); }
      message.success('Instance saved');
      setInstanceModalOpen(false);
      load();
    } catch { message.error('Failed to save instance'); }
  };

  const modelColumns = [
    { title: 'Name', dataIndex: 'name', render: (v: string, r: ModelEntry) => <a onClick={() => openEditModel(r)}>{v}</a> },
    { title: 'Display Name', dataIndex: 'display_name' },
    { title: 'Provider', dataIndex: 'provider', render: (v: string) => <Tag color="blue">{v}</Tag> },
    { title: 'Context', dataIndex: 'context_window', width: 100, render: (v: number) => v ? `${(v / 1000).toFixed(0)}k` : '-' },
    { title: 'Featured', dataIndex: 'is_featured', width: 80, render: (v: boolean) => v ? <Tag color="gold">Featured</Tag> : null },
    { title: 'Status', key: 'status', width: 100, render: (_: any, r: ModelEntry) => <Switch size="small" checked={r.is_active} onChange={() => toggleModel(r)} /> },
    { title: '', key: 'actions', width: 80, render: (_: any, r: ModelEntry) => (
      <Popconfirm title="Delete?" onConfirm={() => api.deleteModel(r.id).then(load)}>
        <Button size="small" danger icon={<DeleteOutlined />} />
      </Popconfirm>
    )},
  ];

  const healthColor = (s: string) => s === 'healthy' ? 'green' : s === 'down' ? 'red' : 'default';

  const instanceColumns = [
    { title: 'Instance', dataIndex: 'instance_name', render: (v: string, r: ProviderInstance) => <a onClick={() => openEditInstance(r)}>{v}</a> },
    { title: 'Model ID', dataIndex: 'model_id', width: 120, render: (v: string) => <Tag>{v.slice(0, 8)}...</Tag> },
    { title: 'Provider', dataIndex: 'provider_type', render: (v: string) => <Tag color="blue">{v}</Tag> },
    { title: 'Priority', dataIndex: 'priority', width: 80 },
    { title: 'Base URL', dataIndex: 'base_url', ellipsis: true, render: (v: string) => v || 'Default' },
    { title: 'Health', dataIndex: 'health_status', width: 80, render: (v: string) => <Tag color={healthColor(v)}>{v}</Tag> },
    { title: 'Status', key: 'status', width: 80, render: (_: any, r: ProviderInstance) => <Tag color={r.is_active ? 'green' : 'red'}>{r.is_active ? 'Active' : 'Off'}</Tag> },
    { title: '', key: 'actions', width: 80, render: (_: any, r: ProviderInstance) => (
      <Popconfirm title="Delete?" onConfirm={() => api.deleteInstance(r.id).then(load)}>
        <Button size="small" danger icon={<DeleteOutlined />} />
      </Popconfirm>
    )},
  ];

  return (
    <div>
      <Title level={4} style={{ marginBottom: 16 }}>Models & Routing</Title>
      <Tabs items={[
        {
          key: 'models',
          label: 'Models',
          children: <>
            <div style={{ marginBottom: 16, textAlign: 'right' }}>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreateModel}>Add Model</Button>
            </div>
            <Table dataSource={models} columns={modelColumns} rowKey="id" loading={loading} size="middle" />
          </>,
        },
        {
          key: 'instances',
          label: 'Provider Instances',
          children: <>
            <div style={{ marginBottom: 16, textAlign: 'right' }}>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreateInstance}>Add Instance</Button>
            </div>
            <Table dataSource={instances} columns={instanceColumns} rowKey="id" loading={loading} size="middle" />
          </>,
        },
      ]} />

      {/* Model Modal */}
      <Modal title={editingModel ? 'Edit Model' : 'Add Model'} open={modelModalOpen} onOk={saveModel} onCancel={() => setModelModalOpen(false)} width={600}>
        <Form form={modelForm} layout="vertical">
          <Form.Item name="name" label="Model Name" rules={[{ required: true }]}><Input placeholder="e.g. gpt-4o" /></Form.Item>
          <Form.Item name="display_name" label="Display Name"><Input placeholder="e.g. GPT-4o" /></Form.Item>
          <Form.Item name="provider" label="Provider" rules={[{ required: true }]}>
            <Select options={[{ value: 'openai', label: 'OpenAI' }, { value: 'anthropic', label: 'Anthropic' }, { value: 'gemini', label: 'Gemini' }]} />
          </Form.Item>
          <Row gutter={16}>
            <Col span={12}><Form.Item name="context_window" label="Context Window"><InputNumber min={0} style={{ width: '100%' }} /></Form.Item></Col>
            <Col span={12}><Form.Item name="max_output" label="Max Output"><InputNumber min={0} style={{ width: '100%' }} /></Form.Item></Col>
          </Row>
          <Form.Item name="sort_order" label="Sort Order"><InputNumber min={0} style={{ width: '100%' }} /></Form.Item>
          <Form.Item name="capabilities" label="Capabilities (comma separated)"><Input placeholder="e.g. vision, tools, reasoning" /></Form.Item>
          <Row gutter={16}>
            <Col span={12}><Form.Item name="is_active" label="Active" valuePropName="checked"><Switch /></Form.Item></Col>
            <Col span={12}><Form.Item name="is_featured" label="Featured" valuePropName="checked"><Switch /></Form.Item></Col>
          </Row>
        </Form>
      </Modal>

      {/* Instance Modal */}
      <Modal title={editingInstance ? 'Edit Instance' : 'Add Instance'} open={instanceModalOpen} onOk={saveInstance} onCancel={() => setInstanceModalOpen(false)} width={600}>
        <Form form={instanceForm} layout="vertical">
          <Form.Item name="model_id" label="Model ID" rules={[{ required: true }]}><Input placeholder="UUID of the model" /></Form.Item>
          <Form.Item name="instance_name" label="Instance Name" rules={[{ required: true }]}><Input placeholder="e.g. openai-us, azure-eastus" /></Form.Item>
          <Form.Item name="provider_type" label="Provider Type" rules={[{ required: true }]}>
            <Select options={[{ value: 'openai', label: 'OpenAI' }, { value: 'anthropic', label: 'Anthropic' }, { value: 'gemini', label: 'Gemini' }]} />
          </Form.Item>
          <Form.Item name="priority" label="Priority (lower = higher)"><InputNumber min={0} style={{ width: '100%' }} /></Form.Item>
          <Form.Item name="base_url" label="Base URL (leave empty for default)"><Input placeholder="https://..." /></Form.Item>
          <Form.Item name="is_active" label="Active" valuePropName="checked"><Switch /></Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
