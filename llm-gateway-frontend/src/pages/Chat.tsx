import React, { useState, useRef, useEffect } from 'react'
import {
  Card, Input, Button, Select, Space, Divider,
  Typography, Tag, Spin, message, Row, Col, Slider
} from 'antd'
import { SendOutlined, ClearOutlined, RobotOutlined, UserOutlined } from '@ant-design/icons'
import { ChatMessage, ChatCompletion } from '../types'
import apiService from '../services/api'
import { useStore } from '../store/useStore'

const { TextArea } = Input
const { Option } = Select
const { Text, Paragraph } = Typography

const Chat: React.FC = () => {
  const { providers } = useStore()
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [inputText, setInputText] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [temperature, setTemperature] = useState(0.7)
  const [maxTokens, setMaxTokens] = useState(1000)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  useEffect(() => {
    if (providers.length > 0 && !selectedModel) {
      const firstOnlineProvider = providers.find(p => p.status === 'online')
      if (firstOnlineProvider) {
        setSelectedModel(firstOnlineProvider.model)
      }
    }
  }, [providers, selectedModel])

  const handleSendMessage = async () => {
    if (!inputText.trim() || !selectedModel) {
      message.warning('请输入消息并选择模型')
      return
    }

    const userMessage: ChatMessage = {
      role: 'user',
      content: inputText.trim(),
      timestamp: new Date()
    }

    setMessages(prev => [...prev, userMessage])
    setInputText('')
    setIsLoading(true)

    try {
      const chatRequest: ChatCompletion = {
        model: selectedModel,
        messages: [...messages, userMessage].map(msg => ({
          role: msg.role,
          content: msg.content
        })),
        maxTokens,
        temperature
      }

      const response = await apiService.chatCompletion(chatRequest)
      
      const assistantMessage: ChatMessage = {
        role: 'assistant',
        content: response.choices?.[0]?.message?.content || '抱歉，没有收到有效响应',
        timestamp: new Date()
      }

      setMessages(prev => [...prev, assistantMessage])
    } catch (error) {
      message.error('发送消息失败，请稍后重试')
      console.error('Chat error:', error)
    } finally {
      setIsLoading(false)
    }
  }

  const handleClearMessages = () => {
    setMessages([])
  }

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSendMessage()
    }
  }

  const renderMessage = (message: ChatMessage, index: number) => {
    const isUser = message.role === 'user'
    return (
      <div
        key={index}
        style={{
          display: 'flex',
          justifyContent: isUser ? 'flex-end' : 'flex-start',
          marginBottom: 16
        }}
      >
        <div
          style={{
            maxWidth: '70%',
            padding: '12px 16px',
            borderRadius: '12px',
            backgroundColor: isUser ? '#1890ff' : '#f5f5f5',
            color: isUser ? '#fff' : '#000'
          }}
        >
          <Space align="start">
            {isUser ? <UserOutlined /> : <RobotOutlined />}
            <div>
              <Paragraph
                style={{
                  margin: 0,
                  color: isUser ? '#fff' : '#000',
                  whiteSpace: 'pre-wrap'
                }}
              >
                {message.content}
              </Paragraph>
              <Text
                type="secondary"
                style={{
                  fontSize: '12px',
                  color: isUser ? 'rgba(255,255,255,0.7)' : undefined
                }}
              >
                {message.timestamp.toLocaleTimeString()}
              </Text>
            </div>
          </Space>
        </div>
      </div>
    )
  }

  return (
    <div style={{ padding: '24px', height: '100%' }}>
      <Row gutter={16} style={{ height: '100%' }}>
        <Col xs={24} lg={18}>
          <Card
            title="聊天测试"
            extra={
              <Button
                icon={<ClearOutlined />}
                onClick={handleClearMessages}
                disabled={messages.length === 0}
              >
                清空对话
              </Button>
            }
            style={{ height: '100%', display: 'flex', flexDirection: 'column' }}
            bodyStyle={{ flex: 1, display: 'flex', flexDirection: 'column' }}
          >
            {/* 消息显示区域 */}
            <div
              style={{
                flex: 1,
                overflow: 'auto',
                marginBottom: 16,
                padding: '16px',
                border: '1px solid #f0f0f0',
                borderRadius: '8px',
                backgroundColor: '#fafafa'
              }}
            >
              {messages.length === 0 ? (
                <div style={{ textAlign: 'center', color: '#999', marginTop: '100px' }}>
                  <RobotOutlined style={{ fontSize: '48px', marginBottom: '16px' }} />
                  <div>开始与AI对话吧！</div>
                </div>
              ) : (
                messages.map(renderMessage)
              )}
              {isLoading && (
                <div style={{ textAlign: 'center', padding: '20px' }}>
                  <Spin />
                  <div style={{ marginTop: '8px', color: '#999' }}>AI正在思考中...</div>
                </div>
              )}
              <div ref={messagesEndRef} />
            </div>

            {/* 输入区域 */}
            <Space.Compact style={{ width: '100%' }}>
              <TextArea
                value={inputText}
                onChange={(e) => setInputText(e.target.value)}
                placeholder="输入你的消息... (Shift+Enter换行，Enter发送)"
                onKeyPress={handleKeyPress}
                autoSize={{ minRows: 1, maxRows: 4 }}
                disabled={isLoading}
              />
              <Button
                type="primary"
                icon={<SendOutlined />}
                onClick={handleSendMessage}
                loading={isLoading}
                disabled={!inputText.trim() || !selectedModel}
              >
                发送
              </Button>
            </Space.Compact>
          </Card>
        </Col>

        <Col xs={24} lg={6}>
          <Card title="设置" size="small">
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text strong>模型选择:</Text>
                <Select
                  value={selectedModel}
                  onChange={setSelectedModel}
                  style={{ width: '100%', marginTop: 8 }}
                  placeholder="选择模型"
                >
                  {providers
                    .filter(p => p.status === 'online')
                    .map(provider => (
                      <Option key={provider.id} value={provider.model}>
                        <Space>
                          {provider.model}
                          <Tag color="green" size="small">{provider.type}</Tag>
                        </Space>
                      </Option>
                    ))}
                </Select>
              </div>

              <Divider />

              <div>
                <Text strong>温度参数: {temperature}</Text>
                <Slider
                  min={0}
                  max={2}
                  step={0.1}
                  value={temperature}
                  onChange={setTemperature}
                  style={{ marginTop: 8 }}
                />
                <Text type="secondary" style={{ fontSize: '12px' }}>
                  控制回答的随机性
                </Text>
              </div>

              <div>
                <Text strong>最大Token数: {maxTokens}</Text>
                <Slider
                  min={100}
                  max={4000}
                  step={100}
                  value={maxTokens}
                  onChange={setMaxTokens}
                  style={{ marginTop: 8 }}
                />
                <Text type="secondary" style={{ fontSize: '12px' }}>
                  控制回答的最大长度
                </Text>
              </div>

              <Divider />

              <div>
                <Text strong>对话统计:</Text>
                <div style={{ marginTop: 8 }}>
                  <Text type="secondary">消息数量: {messages.length}</Text>
                </div>
                <div>
                  <Text type="secondary">
                    Token使用: ~{messages.reduce((acc, msg) => acc + msg.content.length, 0)}
                  </Text>
                </div>
              </div>
            </Space>
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default Chat