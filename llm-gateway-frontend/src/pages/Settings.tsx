import React, { useEffect, useState } from 'react'
import {
  Card, Form, Input, InputNumber, Switch, Button, Select,
  Space, Divider, Typography, Row, Col, message, Tabs
} from 'antd'
import { SaveOutlined, ReloadOutlined } from '@ant-design/icons'
import { SystemConfig } from '../types'
import apiService from '../services/api'
import { useStore } from '../store/useStore'

const { Title, Text } = Typography
const { Option } = Select
const { TabPane } = Tabs

const Settings: React.FC = () => {
  const { systemConfig, fetchSystemConfig } = useStore()
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    fetchSystemConfig()
  }, [fetchSystemConfig])

  useEffect(() => {
    if (systemConfig) {
      form.setFieldsValue(systemConfig)
    }
  }, [systemConfig, form])

  const handleSave = async () => {
    try {
      setLoading(true)
      const values = await form.validateFields()
      await apiService.updateSystemConfig(values)
      await fetchSystemConfig()
      message.success('配置保存成功')
    } catch (error) {
      message.error('配置保存失败')
    } finally {
      setLoading(false)
    }
  }

  const handleReset = () => {
    if (systemConfig) {
      form.setFieldsValue(systemConfig)
      message.info('已重置为当前配置')
    }
  }

  return (
    <div style={{ padding: '24px' }}>
      <Card>
        <Title level={3}>系统设置</Title>
        
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSave}
        >
          <Tabs defaultActiveKey="server">
            <TabPane tab="服务配置" key="server">
              <Row gutter={24}>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['server', 'host']}
                    label="服务器地址"
                    rules={[{ required: true, message: '请输入服务器地址' }]}
                  >
                    <Input placeholder="0.0.0.0" />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['server', 'port']}
                    label="服务器端口"
                    rules={[{ required: true, message: '请输入服务器端口' }]}
                  >
                    <InputNumber
                      min={1}
                      max={65535}
                      style={{ width: '100%' }}
                      placeholder="8080"
                    />
                  </Form.Item>
                </Col>
                <Col xs={24}>
                  <Form.Item
                    name={['server', 'cors']}
                    label="启用CORS"
                    valuePropName="checked"
                  >
                    <Switch />
                  </Form.Item>
                  <Text type="secondary">
                    允许跨域请求，用于前后端分离部署
                  </Text>
                </Col>
              </Row>
            </TabPane>

            <TabPane tab="路由配置" key="routing">
              <Row gutter={24}>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['routing', 'strategy']}
                    label="路由策略"
                    rules={[{ required: true, message: '请选择路由策略' }]}
                  >
                    <Select>
                      <Option value="round-robin">轮询</Option>
                      <Option value="weighted">加权轮询</Option>
                      <Option value="least-latency">最低延迟</Option>
                      <Option value="cost-optimized">成本优化</Option>
                      <Option value="random">随机</Option>
                    </Select>
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['routing', 'fallbackEnabled']}
                    label="启用故障转移"
                    valuePropName="checked"
                  >
                    <Switch />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['routing', 'circuitBreakerEnabled']}
                    label="启用断路器"
                    valuePropName="checked"
                  >
                    <Switch />
                  </Form.Item>
                </Col>
              </Row>

              <Divider />
              <Title level={5}>重试策略</Title>
              <Row gutter={24}>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['routing', 'retryPolicy', 'maxRetries']}
                    label="最大重试次数"
                  >
                    <InputNumber min={0} max={10} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['routing', 'retryPolicy', 'retryDelay']}
                    label="重试延迟(ms)"
                  >
                    <InputNumber min={100} max={10000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>
            </TabPane>

            <TabPane tab="监控配置" key="monitoring">
              <Row gutter={24}>
                <Col xs={24}>
                  <Form.Item
                    name={['monitoring', 'enabled']}
                    label="启用监控"
                    valuePropName="checked"
                  >
                    <Switch />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['monitoring', 'interval']}
                    label="监控间隔(秒)"
                  >
                    <InputNumber min={1} max={3600} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>

              <Divider />
              <Title level={5}>告警阈值</Title>
              <Row gutter={24}>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['monitoring', 'alerts', 'errorRate']}
                    label="错误率阈值(%)"
                  >
                    <InputNumber
                      min={0}
                      max={100}
                      step={0.1}
                      style={{ width: '100%' }}
                    />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['monitoring', 'alerts', 'responseTime']}
                    label="响应时间阈值(ms)"
                  >
                    <InputNumber min={1} max={10000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>
            </TabPane>

            <TabPane tab="限流配置" key="ratelimit">
              <Row gutter={24}>
                <Col xs={24}>
                  <Form.Item
                    name={['rateLimit', 'enabled']}
                    label="启用限流"
                    valuePropName="checked"
                  >
                    <Switch />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    name={['rateLimit', 'requestsPerMinute']}
                    label="每分钟请求数限制"
                  >
                    <InputNumber min={1} max={100000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>
            </TabPane>
          </Tabs>

          <Divider />
          
          <Space>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              loading={loading}
              onClick={handleSave}
            >
              保存配置
            </Button>
            <Button
              icon={<ReloadOutlined />}
              onClick={handleReset}
            >
              重置
            </Button>
          </Space>
        </Form>
      </Card>
    </div>
  )
}

export default Settings