import React, { useEffect, useState } from 'react'
import { Card, Row, Col, Select, DatePicker, Statistic, Table } from 'antd'
import {
  LineChart, Line, AreaChart, Area, BarChart, Bar,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  PieChart, Pie, Cell
} from 'recharts'
import { useStore } from '../store/useStore'

const { RangePicker } = DatePicker
const { Option } = Select

const Metrics: React.FC = () => {
  const { fetchMetrics } = useStore()
  const [timeRange, setTimeRange] = useState('24h')
  const [selectedProvider, setSelectedProvider] = useState('all')

  useEffect(() => {
    fetchMetrics()
  }, [fetchMetrics])

  // 模拟指标数据
  const requestData = [
    { time: '00:00', requests: 45, tokens: 1234, latency: 42 },
    { time: '04:00', requests: 23, tokens: 876, latency: 38 },
    { time: '08:00', requests: 156, tokens: 4521, latency: 55 },
    { time: '12:00', requests: 234, tokens: 6789, latency: 48 },
    { time: '16:00', requests: 198, tokens: 5432, latency: 52 },
    { time: '20:00', requests: 167, tokens: 4876, latency: 45 },
  ]

  const providerData = [
    { name: 'OpenAI', value: 45, color: '#1890ff' },
    { name: 'Claude', value: 30, color: '#52c41a' },
    { name: '百度文心', value: 15, color: '#faad14' },
    { name: '其他', value: 10, color: '#f5222d' },
  ]

  const errorData = [
    { time: '00:00', rate: 0.02 },
    { time: '04:00', rate: 0.01 },
    { time: '08:00', rate: 0.05 },
    { time: '12:00', rate: 0.03 },
    { time: '16:00', rate: 0.04 },
    { time: '20:00', rate: 0.02 },
  ]

  const topRequests = [
    { model: 'gpt-3.5-turbo', requests: 1234, avgLatency: 45, errorRate: 0.02 },
    { model: 'gpt-4', requests: 892, avgLatency: 78, errorRate: 0.01 },
    { model: 'claude-3', requests: 567, avgLatency: 52, errorRate: 0.03 },
    { model: 'text-davinci-003', requests: 345, avgLatency: 65, errorRate: 0.02 },
  ]

  const columns = [
    {
      title: '模型',
      dataIndex: 'model',
      key: 'model',
    },
    {
      title: '请求数',
      dataIndex: 'requests',
      key: 'requests',
      render: (value: number) => value.toLocaleString(),
    },
    {
      title: '平均延迟',
      dataIndex: 'avgLatency',
      key: 'avgLatency',
      render: (value: number) => `${value}ms`,
    },
    {
      title: '错误率',
      dataIndex: 'errorRate',
      key: 'errorRate',
      render: (value: number) => `${(value * 100).toFixed(2)}%`,
    },
  ]

  return (
    <div style={{ padding: '24px' }}>
      {/* 控制栏 */}
      <Card style={{ marginBottom: 16 }}>
        <Row gutter={16} align="middle">
          <Col>
            <span>时间范围:</span>
            <Select
              value={timeRange}
              onChange={setTimeRange}
              style={{ width: 120, marginLeft: 8 }}
            >
              <Option value="1h">最近1小时</Option>
              <Option value="24h">最近24小时</Option>
              <Option value="7d">最近7天</Option>
              <Option value="30d">最近30天</Option>
            </Select>
          </Col>
          <Col>
            <span>提供商:</span>
            <Select
              value={selectedProvider}
              onChange={setSelectedProvider}
              style={{ width: 120, marginLeft: 8 }}
            >
              <Option value="all">全部</Option>
              <Option value="openai">OpenAI</Option>
              <Option value="claude">Claude</Option>
              <Option value="baidu">百度文心</Option>
            </Select>
          </Col>
        </Row>
      </Card>

      {/* 核心指标 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="总请求数"
              value={12340}
              suffix="次"
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="总Token数"
              value={1234567}
              suffix="个"
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="平均延迟"
              value={48}
              suffix="ms"
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="错误率"
              value={2.1}
              suffix="%"
              precision={1}
            />
          </Card>
        </Col>
      </Row>

      {/* 图表 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} lg={12}>
          <Card title="请求量趋势">
            <ResponsiveContainer width="100%" height={300}>
              <AreaChart data={requestData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="time" />
                <YAxis />
                <Tooltip />
                <Area
                  type="monotone"
                  dataKey="requests"
                  stroke="#1890ff"
                  fill="#1890ff"
                  fillOpacity={0.1}
                />
              </AreaChart>
            </ResponsiveContainer>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="Token使用量">
            <ResponsiveContainer width="100%" height={300}>
              <BarChart data={requestData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="time" />
                <YAxis />
                <Tooltip />
                <Bar dataKey="tokens" fill="#52c41a" />
              </BarChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} lg={12}>
          <Card title="提供商分布">
            <ResponsiveContainer width="100%" height={300}>
              <PieChart>
                <Pie
                  data={providerData}
                  cx="50%"
                  cy="50%"
                  outerRadius={100}
                  dataKey="value"
                  label={({ name, percent }) => `${name} ${(percent * 100).toFixed(0)}%`}
                >
                  {providerData.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="错误率趋势">
            <ResponsiveContainer width="100%" height={300}>
              <LineChart data={errorData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="time" />
                <YAxis domain={[0, 'dataMax']} />
                <Tooltip />
                <Line
                  type="monotone"
                  dataKey="rate"
                  stroke="#ff4d4f"
                  strokeWidth={2}
                />
              </LineChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>

      {/* 模型使用统计 */}
      <Row>
        <Col span={24}>
          <Card title="模型使用统计">
            <Table
              columns={columns}
              dataSource={topRequests}
              rowKey="model"
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default Metrics