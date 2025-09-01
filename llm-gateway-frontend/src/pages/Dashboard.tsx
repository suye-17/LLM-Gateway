import React, { useEffect } from 'react'
import { Row, Col, Card, Statistic, Progress, Table, Tag, Space, Alert } from 'antd'
import {
  ApiOutlined,
  ThunderboltOutlined,
  ClockCircleOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  ArrowUpOutlined,
  ArrowDownOutlined
} from '@ant-design/icons'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, AreaChart, Area } from 'recharts'
import { useStore } from '../store/useStore'

const Dashboard: React.FC = () => {
  const { providers, metrics, fetchProviders, fetchMetrics, isLoading } = useStore()

  useEffect(() => {
    fetchProviders()
    fetchMetrics()
    
    // 每30秒刷新一次数据
    const interval = setInterval(() => {
      fetchMetrics()
    }, 30000)
    
    return () => clearInterval(interval)
  }, [fetchProviders, fetchMetrics])

  // 模拟实时数据
  const performanceData = [
    { time: '09:00', requests: 120, latency: 45 },
    { time: '09:30', requests: 150, latency: 52 },
    { time: '10:00', requests: 180, latency: 38 },
    { time: '10:30', requests: 200, latency: 42 },
    { time: '11:00', requests: 220, latency: 35 },
    { time: '11:30', requests: 190, latency: 48 },
  ]

  const providerColumns = [
    {
      title: '提供商',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: any) => (
        <Space>
          <ApiOutlined />
          {name}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => {
        const statusConfig = {
          online: { color: 'success', text: '在线' },
          offline: { color: 'error', text: '离线' },
          warning: { color: 'warning', text: '警告' }
        }
        const config = statusConfig[status as keyof typeof statusConfig] || statusConfig.offline
        return <Tag color={config.color}>{config.text}</Tag>
      },
    },
    {
      title: '请求数',
      dataIndex: ['stats', 'totalRequests'],
      key: 'requests',
      render: (value: number) => value.toLocaleString(),
    },
    {
      title: '成功率',
      dataIndex: ['stats', 'successRate'],
      key: 'successRate',
      render: (value: number) => (
        <Progress
          percent={value * 100}
          size="small"
          status={value > 0.95 ? 'success' : value > 0.9 ? 'normal' : 'exception'}
        />
      ),
    },
    {
      title: '平均延迟',
      dataIndex: ['health', 'responseTime'],
      key: 'responseTime',
      render: (value: number) => `${value}ms`,
    },
  ]

  const onlineProviders = providers.filter(p => p.status === 'online').length
  const totalProviders = providers.length
  const avgResponseTime = metrics?.avgResponseTime || 0
  const successRate = metrics?.successRate || 0

  return (
    <div style={{ padding: '24px' }}>
      {/* 系统状态警告 */}
      {successRate < 0.9 && (
        <Alert
          message="系统警告"
          description="检测到部分提供商响应异常，请及时处理"
          type="warning"
          showIcon
          style={{ marginBottom: 24 }}
        />
      )}

      {/* 关键指标卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} md={6}>
          <Card className="dashboard-card">
            <Statistic
              title="在线提供商"
              value={onlineProviders}
              suffix={`/ ${totalProviders}`}
              prefix={<ApiOutlined />}
              valueStyle={{ color: onlineProviders === totalProviders ? '#3f8600' : '#cf1322' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card className="dashboard-card">
            <Statistic
              title="总请求数"
              value={metrics?.totalRequests || 0}
              prefix={<ThunderboltOutlined />}
              suffix={<ArrowUpOutlined style={{ color: '#3f8600' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card className="dashboard-card">
            <Statistic
              title="平均延迟"
              value={avgResponseTime}
              suffix="ms"
              prefix={<ClockCircleOutlined />}
              valueStyle={{ color: avgResponseTime < 100 ? '#3f8600' : '#cf1322' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card className="dashboard-card">
            <Statistic
              title="成功率"
              value={successRate * 100}
              precision={2}
              suffix="%"
              prefix={<CheckCircleOutlined />}
              valueStyle={{ color: successRate > 0.95 ? '#3f8600' : '#cf1322' }}
            />
          </Card>
        </Col>
      </Row>

      {/* 性能图表 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} lg={12}>
          <Card title="请求量趋势" className="dashboard-card">
            <ResponsiveContainer width="100%" height={300}>
              <AreaChart data={performanceData}>
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
          <Card title="响应延迟" className="dashboard-card">
            <ResponsiveContainer width="100%" height={300}>
              <LineChart data={performanceData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="time" />
                <YAxis />
                <Tooltip />
                <Line
                  type="monotone"
                  dataKey="latency"
                  stroke="#52c41a"
                  strokeWidth={2}
                />
              </LineChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>

      {/* 提供商状态表格 */}
      <Row>
        <Col span={24}>
          <Card title="提供商状态" className="dashboard-card">
            <Table
              columns={providerColumns}
              dataSource={providers}
              rowKey="id"
              loading={isLoading}
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default Dashboard