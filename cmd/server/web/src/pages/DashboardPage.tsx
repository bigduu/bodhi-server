import { useEffect, useState } from 'react';
import { Row, Col, Card, Statistic, Typography, Select, Space, Button, message } from 'antd';
import { ApiOutlined, TeamOutlined, ThunderboltOutlined, WarningOutlined, CopyOutlined, LinkOutlined } from '@ant-design/icons';
import { LineChart, Line, BarChart, Bar, PieChart, Pie, Cell, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import { api } from '../services/api';
import type { DashboardData } from '../types';

const { Title, Text } = Typography;
const COLORS = ['#1677ff', '#52c41a', '#faad14', '#ff4d4f', '#722ed1'];

function copyToClipboard(text: string) {
  navigator.clipboard.writeText(text).then(() => message.success('Copied')).catch(() => message.error('Copy failed'));
}

export default function DashboardPage() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [days, setDays] = useState(7);

  useEffect(() => {
    api.usageSummary(days).then(setData).catch(() => {});
  }, [days]);

  if (!data) return null;

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <Title level={4} style={{ margin: 0 }}>Dashboard</Title>
        <Select value={days} onChange={setDays} style={{ width: 120 }} options={[
          { value: 7, label: 'Last 7 days' },
          { value: 30, label: 'Last 30 days' },
          { value: 90, label: 'Last 90 days' },
        ]} />
      </div>

      <Card size="small" style={{ marginBottom: 16 }} bodyStyle={{ padding: '8px 16px' }}>
        <Space wrap size="middle">
          <LinkOutlined style={{ color: '#0d9488' }} />
          <Text strong>Endpoint:</Text>
          <Text code>{window.location.origin}</Text>
          <Button size="small" icon={<CopyOutlined />} onClick={() => copyToClipboard(window.location.origin)}>Copy</Button>
          <Text type="secondary">|</Text>
          <Text strong>Proxy:</Text>
          <Text code>{window.location.origin}/proxy/v1/</Text>
          <Button size="small" icon={<CopyOutlined />} onClick={() => copyToClipboard(window.location.origin + '/proxy/v1/')}>Copy</Button>
        </Space>
      </Card>

      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col span={6}>
          <Card><Statistic title="Total Requests" value={data.summary.total_requests} prefix={<ApiOutlined />} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="Active Users" value={data.summary.active_users} prefix={<TeamOutlined />} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="Total Tokens" value={data.summary.total_tokens} prefix={<ThunderboltOutlined />} /></Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="Avg Response (ms)" value={Math.round(data.summary.avg_duration_ms)} prefix={<WarningOutlined />}
              valueStyle={{ color: data.summary.avg_duration_ms > 3000 ? '#cf1322' : '#3f8600' }} />
          </Card>
        </Col>
      </Row>

      <Row gutter={16}>
        <Col span={16}>
          <Card title="Request Trend">
            <ResponsiveContainer width="100%" height={300}>
              <LineChart data={data.daily}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="date" />
                <YAxis />
                <Tooltip />
                <Legend />
                <Line type="monotone" dataKey="requests" stroke="#1677ff" strokeWidth={2} />
              </LineChart>
            </ResponsiveContainer>
          </Card>
        </Col>
        <Col span={8}>
          <Card title="By Provider">
            <ResponsiveContainer width="100%" height={300}>
              <PieChart>
                <Pie data={data.by_provider} dataKey="requests" nameKey="provider" cx="50%" cy="50%" outerRadius={100} label>
                  {data.by_provider.map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="Tokens by Provider">
            <ResponsiveContainer width="100%" height={250}>
              <BarChart data={data.by_provider}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="provider" />
                <YAxis />
                <Tooltip />
                <Bar dataKey="tokens" fill="#1677ff" />
              </BarChart>
            </ResponsiveContainer>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="Top Users">
            <ResponsiveContainer width="100%" height={250}>
              <BarChart data={data.by_user.slice(0, 10)} layout="vertical">
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis type="number" />
                <YAxis type="category" dataKey="username" width={100} />
                <Tooltip />
                <Bar dataKey="requests" fill="#52c41a" />
              </BarChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>
    </div>
  );
}
