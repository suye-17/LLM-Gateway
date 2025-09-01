import React, { useEffect, useState } from 'react'
import {
  Card,
  Table,
  Button,
  Space,
  Tag,
  Modal,
  Form,
  Input,
  Select,
  InputNumber,
  Switch,
  message,
  Popconfirm,
  Progress
} from 'antd'
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  PlayCircleOutlined,
  StopOutlined,
  ApiOutlined
} from '@ant-design/icons'
import { useStore } from '../store/useStore'
import { Provider } from '../types'
import apiService from '../services/api'

const { Option } = Select

const Providers: React.FC = () => {
  const { providers, fetchProviders, isLoading } = useStore()
  const [modalVisible, setModalVisible] = useState(false)
  const [editingProvider, setEditingProvider] = useState<Provider | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchProviders()
  }, [fetchProviders])

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: Provider) => (
        <Space>
          <ApiOutlined />
          <span>{name}</span>
          <Tag color="blue">{record.type}</Tag>
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
      title: '模型',
      dataIndex: 'model',
      key: 'model',
    },
    {
      title: '端点',
      dataIndex: 'endpoint',
      key: 'endpoint',
      ellipsis: true,
    },
    {
      title: '速率限制',
      key: 'rateLimit',
      render: (_: any, record: Provider) => (
        <div>
          <div>请求: {record.rateLimits.requestsPerMinute}/min</div>
          <div>Token: {record.rateLimits.tokensPerMinute}/min</div>
        </div>
      ),
    },
    {
      title: '健康状况',
      key: 'health',
      render: (_: any, record: Provider) => (
        <div>
          <div>延迟: {record.health.responseTime}ms</div>
          <Progress
            percent={100 - record.health.errorRate * 100}
            size="small"
            status={record.health.errorRate < 0.05 ? 'success' : 'exception'}
          />
        </div>
      ),
    },
    {
      title: '统计',
      key: 'stats',
      render: (_: any, record: Provider) => (
        <div>
          <div>请求: {record.stats.totalRequests.toLocaleString()}</div>
          <div>Token: {record.stats.totalTokens.toLocaleString()}</div>
          <div>成功率: {(record.stats.successRate * 100).toFixed(1)}%</div>
        </div>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      render: (_: any, record: Provider) => (
        <Space>
          <Button
            type="text"
            icon={<PlayCircleOutlined />}
            onClick={() => handleTestProvider(record.id)}
            size="small"
          >
            测试
          </Button>
          <Button
            type="text"
            icon={<EditOutlined />}
            onClick={() => handleEditProvider(record)}
            size="small"
          >
            编辑
          </Button>
          <Popconfirm
            title="确认删除此提供商？"
            onConfirm={() => handleDeleteProvider(record.id)}
          >
            <Button
              type="text"
              danger
              icon={<DeleteOutlined />}
              size="small"
            >
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const handleAddProvider = () => {
    setEditingProvider(null)
    form.resetFields()
    setModalVisible(true)
  }

  const handleEditProvider = (provider: Provider) => {
    setEditingProvider(provider)
    form.setFieldsValue(provider)
    setModalVisible(true)
  }

  const handleTestProvider = async (providerId: string) => {
    try {
      const success = await apiService.testProvider(providerId)
      if (success) {
        message.success('提供商连接测试成功')
      } else {
        message.error('提供商连接测试失败')
      }
    } catch (error) {
      message.error('测试失败')
    }
  }

  const handleDeleteProvider = async (providerId: string) => {
    try {
      // TODO: 实现删除API
      message.success('提供商删除成功')
      fetchProviders()
    } catch (error) {
      message.error('删除失败')
    }
  }

  const handleModalOk = async () => {
    try {
      const values = await form.validateFields()
      
      if (editingProvider) {
        await apiService.updateProvider(editingProvider.id, values)
      } else {
        // TODO: 实现创建API
        message.success('提供商创建成功')
      }
      
      setModalVisible(false)
      fetchProviders()
    } catch (error) {
      // 表单验证失败或API错误
    }
  }

  return (
    <div style={{ padding: '24px' }}>
      <Card
        title="模型提供商管理"
        extra={
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={handleAddProvider}
          >
            添加提供商
          </Button>
        }
      >
        <Table
          columns={columns}
          dataSource={providers}
          rowKey="id"
          loading={isLoading}
          scroll={{ x: 1200 }}
        />
      </Card>

      <Modal
        title={editingProvider ? '编辑提供商' : '添加提供商'}
        open={modalVisible}
        onOk={handleModalOk}
        onCancel={() => setModalVisible(false)}
        width={600}
      >
        <Form
          form={form}
          layout="vertical"
          initialValues={{
            type: 'openai',
            maxTokens: 4096,
            timeout: 30000,
          }}
        >
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, message: '请输入提供商名称' }]}
          >
            <Input placeholder="输入提供商名称" />
          </Form.Item>

          <Form.Item
            name="type"
            label="类型"
            rules={[{ required: true, message: '请选择提供商类型' }]}
          >
            <Select>
              <Option value="openai">OpenAI</Option>
              <Option value="claude">Claude</Option>
              <Option value="baidu">百度文心</Option>
              <Option value="custom">自定义</Option>
            </Select>
          </Form.Item>

          <Form.Item
            name="endpoint"
            label="API端点"
            rules={[{ required: true, message: '请输入API端点' }]}
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>

          <Form.Item
            name="apiKey"
            label="API密钥"
            rules={[{ required: true, message: '请输入API密钥' }]}
          >
            <Input.Password placeholder="输入API密钥" />
          </Form.Item>

          <Form.Item
            name="model"
            label="模型名称"
            rules={[{ required: true, message: '请输入模型名称' }]}
          >
            <Input placeholder="gpt-3.5-turbo" />
          </Form.Item>

          <Form.Item
            name="maxTokens"
            label="最大Token数"
          >
            <InputNumber min={1} max={32768} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="timeout"
            label="超时时间(ms)"
          >
            <InputNumber min={1000} max={300000} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Providers