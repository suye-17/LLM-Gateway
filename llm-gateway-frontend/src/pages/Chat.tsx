import React, { useState, useRef, useEffect } from 'react'
import {
  Card, Input, Button, Select, Space, Divider,
  Typography, Tag, Spin, message, Row, Col, Slider, Switch
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
  const [selectedModel, setSelectedModel] = useState('gpt-4')
  const [isLoading, setIsLoading] = useState(false)
  const [temperature, setTemperature] = useState(0.7)
  const [maxTokens, setMaxTokens] = useState(1000)
  const [streamMode, setStreamMode] = useState(true) // 默认开启流式输出
  const [currentStreamMessage, setCurrentStreamMessage] = useState('')
  const messagesEndRef = useRef<HTMLDivElement>(null)

  // 演示用的模型选项（对应智能路由系统支持的模型）
  const availableModels = [
    { value: 'gpt-4', label: 'GPT-4 (OpenAI)', provider: 'openai' },
    { value: 'gpt-3.5-turbo', label: 'GPT-3.5 Turbo (OpenAI)', provider: 'openai' },
    { value: 'claude-3', label: 'Claude-3 (Anthropic)', provider: 'anthropic' },
    { value: 'claude-3-sonnet', label: 'Claude-3 Sonnet (Anthropic)', provider: 'anthropic' },
    { value: 'ernie-bot', label: '文心一言 (Baidu)', provider: 'baidu' },
    { value: 'ernie-bot-4', label: '文心一言 4.0 (Baidu)', provider: 'baidu' },
    { value: 'glm-4.5', label: 'GLM-4.5 (智谱AI)', provider: 'zhipu' },
    { value: 'glm-4.5v', label: 'GLM-4.5V (智谱AI)', provider: 'zhipu' },
    { value: 'glm-4.5-air', label: 'GLM-4.5-Air (智谱AI)', provider: 'zhipu' },
  ]

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  // 注释掉原来的provider依赖逻辑，使用静态模型列表
  // useEffect(() => {
  //   if (providers.length > 0 && !selectedModel) {
  //     const firstOnlineProvider = providers.find(p => p.status === 'online')
  //     if (firstOnlineProvider) {
  //       setSelectedModel(firstOnlineProvider.model)
  //     }
  //   }
  // }, [providers, selectedModel])

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
    setCurrentStreamMessage('')

    try {
      const chatRequest: ChatCompletion = {
        model: selectedModel,
        messages: [...messages, userMessage].map(msg => ({
          role: msg.role,
          content: msg.content,
          timestamp: msg.timestamp
        })),
        maxTokens,
        temperature
      }

      if (streamMode) {
        // 流式输出模式
        let fullContent = ''
        
        // 添加一个空的助手消息用于流式显示
        setMessages(prev => [...prev, {
          role: 'assistant',
          content: '',
          timestamp: new Date()
        }])

        await apiService.chatCompletionStream(
          chatRequest,
          (chunk: string) => {
            // 接收到新的文本块
            fullContent += chunk
            setMessages(prev => {
              const newMessages = [...prev]
              // 更新最后一条消息（助手消息）
              const lastIndex = newMessages.length - 1
              newMessages[lastIndex] = {
                ...newMessages[lastIndex],
                content: fullContent
              }
              return newMessages
            })
          },
          () => {
            // 流式输出完成
            setIsLoading(false)
          }
        )
      } else {
        // 传统一次性输出模式
        const response = await apiService.chatCompletion(chatRequest)
        
        const assistantMessage: ChatMessage = {
          role: 'assistant',
          content: response.choices?.[0]?.message?.content || '抱歉，没有收到有效响应',
          timestamp: new Date()
        }

        setMessages(prev => [...prev, assistantMessage])
      }
    } catch (error) {
      message.error('发送消息失败，请稍后重试')
      console.error('Chat error:', error)
    } finally {
      setIsLoading(false)
      setCurrentStreamMessage('')
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
                  {availableModels.map(model => (
                    <Option key={model.value} value={model.value}>
                      <Space>
                        {model.label}
                        <Tag 
                          color={
                            model.provider === 'openai' ? 'blue' :
                            model.provider === 'anthropic' ? 'green' :
                            model.provider === 'baidu' ? 'orange' : 'default'
                          } 

                        >
                          {model.provider}
                        </Tag>
                      </Space>
                    </Option>
                  ))}
                </Select>
              </div>

              <Divider />

              <div>
                <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                  <Text strong>流式输出:</Text>
                  <Switch 
                    checked={streamMode}
                    onChange={setStreamMode}
                    checkedChildren="开启"
                    unCheckedChildren="关闭"
                  />
                </Space>
                <div style={{ marginTop: 4, fontSize: 12, color: '#999' }}>
                  {streamMode ? '✨ AI将逐字显示回复' : '📝 AI完成思考后一次性显示'}
                </div>
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