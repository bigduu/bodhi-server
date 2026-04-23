import { useState, useEffect, useCallback } from 'react';
import { Card, Table, Statistic, Row, Col, Button, Modal, Form, InputNumber, message, Tag, Typography } from 'antd';
import { DollarOutlined, DownloadOutlined } from '@ant-design/icons';
import { api } from '../services/api';
import type { BillingCurrent, BillingReport, BillingByModel } from '../types';

const { Text } = Typography;

export default function BillingPage() {
  const [current, setCurrent] = useState<BillingCurrent | null>(null);
  const [reports, setReports] = useState<BillingReport[]>([]);
  const [loading, setLoading] = useState(true);
  const [balanceModalOpen, setBalanceModalOpen] = useState(false);
  const [balanceForm] = Form.useForm();
  const [balanceTarget, setBalanceTarget] = useState('');

  const fetchCurrent = useCallback(() => {
    api.billingCurrent().then(setCurrent).catch(() => {});
  }, []);

  const fetchReports = useCallback(() => {
    api.billingReports().then(setReports).catch(() => {});
  }, []);

  useEffect(() => {
    setLoading(true);
    Promise.all([fetchCurrent(), fetchReports()]).finally(() => setLoading(false));
  }, [fetchCurrent, fetchReports]);

  const handleDownloadCSV = async (year: number, month: number) => {
    try {
      const token = localStorage.getItem('access_token');
      const res = await fetch(`/api/v1/billing/reports/${year}/${month}/csv`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!res.ok) throw new Error('download failed');
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `billing-${year}-${month}.csv`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      message.error('Failed to download CSV');
    }
  };

  const handleAddBalance = async () => {
    try {
      const values = await balanceForm.validateFields();
      await api.addBalance(balanceTarget, values.amount_cents);
      message.success('Balance updated');
      setBalanceModalOpen(false);
      balanceForm.resetFields();
      fetchCurrent();
    } catch {
      message.error('Failed to update balance');
    }
  };

  const fmtCents = (cents: number) => `$${(cents / 100).toFixed(2)}`;
  const fmtTokens = (t: number) => t >= 1000000 ? `${(t / 1000000).toFixed(2)}M` : t >= 1000 ? `${(t / 1000).toFixed(1)}K` : String(t);

  const modelColumns = [
    { title: 'Provider', dataIndex: 'provider', key: 'provider', render: (v: string) => <Tag color={v === 'openai' ? 'green' : v === 'anthropic' ? 'orange' : 'blue'}>{v}</Tag> },
    { title: 'Model', dataIndex: 'model', key: 'model' },
    { title: 'Requests', dataIndex: 'requests', key: 'requests' },
    { title: 'Input Tokens', dataIndex: 'input_tokens', key: 'input_tokens', render: fmtTokens },
    { title: 'Output Tokens', dataIndex: 'output_tokens', key: 'output_tokens', render: fmtTokens },
    { title: 'Cost', dataIndex: 'cost_cents', key: 'cost_cents', render: fmtCents },
  ];

  const reportColumns = [
    { title: 'Period', key: 'period', render: (_: unknown, r: BillingReport) => {
      const start = new Date(r.period_start);
      return `${start.getFullYear()}-${String(start.getMonth() + 1).padStart(2, '0')}`;
    }},
    { title: 'Requests', dataIndex: 'total_requests', key: 'total_requests' },
    { title: 'Input Tokens', dataIndex: 'total_input_tokens', key: 'total_input_tokens', render: fmtTokens },
    { title: 'Output Tokens', dataIndex: 'total_output_tokens', key: 'total_output_tokens', render: fmtTokens },
    { title: 'Cost', dataIndex: 'total_cost_cents', key: 'total_cost_cents', render: fmtCents },
    { title: 'Action', key: 'action', render: (_: unknown, r: BillingReport) => {
      const start = new Date(r.period_start);
      return <Button size="small" icon={<DownloadOutlined />} onClick={() => handleDownloadCSV(start.getFullYear(), start.getMonth() + 1)}>CSV</Button>;
    }},
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      <Row gutter={16}>
        <Col span={6}>
          <Card><Statistic title="Balance" value={current ? current.balance_cents / 100 : 0} precision={2} prefix="$" loading={loading} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="Requests (MTD)" value={current?.current?.total_requests ?? 0} loading={loading} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="Tokens (MTD)" value={current ? fmtTokens(current.current.total_input_tokens + current.current.total_output_tokens) : '0'} loading={loading} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="Cost (MTD)" value={current ? current.current.total_cost_cents / 100 : 0} precision={2} prefix="$" loading={loading} /></Card>
        </Col>
      </Row>

      <Card title="Usage by Model" size="small">
        <Table<BillingByModel>
          dataSource={current?.by_model ?? []}
          columns={modelColumns}
          rowKey={(r) => `${r.provider}-${r.model}`}
          pagination={false}
          size="small"
          loading={loading}
        />
      </Card>

      <Card
        title="Monthly Reports"
        size="small"
        extra={
          <Button icon={<DollarOutlined />} onClick={() => { setBalanceTarget(''); setBalanceModalOpen(true); }}>Add Balance</Button>
        }
      >
        <Table<BillingReport>
          dataSource={reports}
          columns={reportColumns}
          rowKey={(r) => r.period_start}
          pagination={false}
          size="small"
          loading={loading}
        />
        {reports.length === 0 && !loading && <Text type="secondary">No billing reports yet. Reports are generated when you view a specific month.</Text>}
      </Card>

      <Modal
        title="Add Balance"
        open={balanceModalOpen}
        onOk={handleAddBalance}
        onCancel={() => setBalanceModalOpen(false)}
      >
        <Form form={balanceForm} layout="vertical">
          <Form.Item name="user_id" label="User ID" rules={[{ required: true, message: 'Required' }]}>
            <InputNumber style={{ width: '100%' }} placeholder="Enter user ID" onChange={(v) => setBalanceTarget(String(v ?? ''))} />
          </Form.Item>
          <Form.Item name="amount_cents" label="Amount (cents)" rules={[{ required: true, message: 'Required' }]}>
            <InputNumber style={{ width: '100%' }} min={1} placeholder="e.g. 1000 = $10.00" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
