import { useEffect, useState } from 'react';
import { Card, Row, Col, Table, Tag, Select, Statistic, Typography } from 'antd';
import { BarChartOutlined, ThunderboltOutlined, WarningOutlined, DollarOutlined } from '@ant-design/icons';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import { api } from '../services/api';
import type { LatencyStats, ModelTokenUsage, ErrorEntry } from '../types';

const { Title } = Typography;

export default function MetricsPage() {
  const [days, setDays] = useState(7);
  const [latency, setLatency] = useState<LatencyStats | null>(null);
  const [tokenUsage, setTokenUsage] = useState<ModelTokenUsage[]>([]);
  const [errors, setErrors] = useState<ErrorEntry[]>([]);
  const [errorTotal, setErrorTotal] = useState(0);
  const [errorPage, setErrorPage] = useState(1);

  useEffect(() => {
    api.latencyStats(days).then(setLatency).catch(() => {});
    api.tokenUsage(days).then(setTokenUsage).catch(() => {});
    api.errorLogs(days).then(res => { setErrors(res.items || []); setErrorTotal(res.total); }).catch(() => {});
  }, [days]);

  const loadErrors = (p: number) => {
    api.errorLogs(days, p).then(res => { setErrors(res.items || []); setErrorTotal(res.total); setErrorPage(p); }).catch(() => {});
  };

  const chartData = tokenUsage.map(t => ({
    name: `${t.provider}/${t.model}`,
    input: t.input_tokens,
    output: t.output_tokens,
    cost: (t.cost_cents / 100).toFixed(2),
    requests: t.requests,
  }));

  const costData = tokenUsage.map(t => ({
    name: `${t.provider}/${t.model}`,
    cost: t.cost_cents / 100,
  }));

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <Title level={4} style={{ margin: 0 }}>Metrics</Title>
        <Select value={days} onChange={setDays} style={{ width: 140 }} options={[
          { value: 1, label: 'Last 24 hours' },
          { value: 7, label: 'Last 7 days' },
          { value: 30, label: 'Last 30 days' },
          { value: 90, label: 'Last 90 days' },
        ]} />
      </div>

      {/* Latency Stats */}
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col span={8}>
          <Card>
            <Statistic title="P50 Latency (ms)" value={latency?.p50 ?? 0} prefix={<ThunderboltOutlined />}
              valueStyle={{ color: (latency?.p50 ?? 0) > 2000 ? '#cf1322' : '#3f8600' }} />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic title="P95 Latency (ms)" value={latency?.p95 ?? 0} prefix={<ThunderboltOutlined />}
              valueStyle={{ color: (latency?.p95 ?? 0) > 5000 ? '#cf1322' : '#faad14' }} />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic title="P99 Latency (ms)" value={latency?.p99 ?? 0} prefix={<ThunderboltOutlined />}
              valueStyle={{ color: (latency?.p99 ?? 0) > 10000 ? '#cf1322' : '#faad14' }} />
          </Card>
        </Col>
      </Row>

      {/* Token Usage by Model */}
      <Card title={<><BarChartOutlined /> Token Usage by Model</>} style={{ marginBottom: 24 }}>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="name" tick={{ fontSize: 11 }} angle={-20} textAnchor="end" height={60} />
            <YAxis />
            <Tooltip />
            <Legend />
            <Bar dataKey="input" stackId="tokens" fill="#1677ff" name="Input Tokens" />
            <Bar dataKey="output" stackId="tokens" fill="#52c41a" name="Output Tokens" />
          </BarChart>
        </ResponsiveContainer>

        <Table
          dataSource={tokenUsage}
          rowKey={(r) => `${r.provider}-${r.model}`}
          size="small"
          style={{ marginTop: 16 }}
          pagination={false}
          columns={[
            { title: 'Provider', dataIndex: 'provider', render: (v: string) => <Tag color="blue">{v}</Tag> },
            { title: 'Model', dataIndex: 'model' },
            { title: 'Requests', dataIndex: 'requests', width: 100 },
            { title: 'Input Tokens', dataIndex: 'input_tokens', width: 120, render: (v: number) => v.toLocaleString() },
            { title: 'Output Tokens', dataIndex: 'output_tokens', width: 120, render: (v: number) => v.toLocaleString() },
            { title: 'Cost ($)', dataIndex: 'cost_cents', width: 100, render: (v: number) => `$${(v / 100).toFixed(2)}` },
          ]}
        />
      </Card>

      {/* Cost Trend */}
      <Card title={<><DollarOutlined /> Cost by Model</>} style={{ marginBottom: 24 }}>
        <ResponsiveContainer width="100%" height={250}>
          <BarChart data={costData}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="name" tick={{ fontSize: 11 }} angle={-20} textAnchor="end" height={60} />
            <YAxis tickFormatter={(v: number) => `$${v}`} />
            <Tooltip formatter={(v) => `$${Number(v).toFixed(2)}`} />
            <Bar dataKey="cost" fill="#faad14" name="Cost ($)" />
          </BarChart>
        </ResponsiveContainer>
      </Card>

      {/* Error Logs */}
      <Card title={<><WarningOutlined /> Error Logs</>}>
        <Table
          dataSource={errors}
          rowKey="id"
          size="small"
          pagination={{ current: errorPage, total: errorTotal, pageSize: 20, onChange: loadErrors }}
          columns={[
            { title: 'Time', dataIndex: 'created_at', width: 180, render: (v: string) => new Date(v).toLocaleString() },
            { title: 'User', dataIndex: 'username', width: 120 },
            { title: 'Provider', dataIndex: 'provider', width: 100, render: (v: string) => <Tag>{v}</Tag> },
            { title: 'Model', dataIndex: 'model', width: 150 },
            { title: 'Status', dataIndex: 'status_code', width: 80, render: (v: number) => <Tag color="red">{v}</Tag> },
            { title: 'Error', dataIndex: 'error_message', ellipsis: true },
          ]}
        />
      </Card>
    </div>
  );
}
