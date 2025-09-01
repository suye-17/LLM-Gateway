// 基础类型定义
export interface Provider {
  id: string
  name: string
  type: 'openai' | 'claude' | 'baidu' | 'custom'
  status: 'online' | 'offline' | 'warning'
  endpoint: string
  apiKey: string
  model: string
  maxTokens: number
  timeout: number
  rateLimits: {
    requestsPerMinute: number
    tokensPerMinute: number
    remainingRequests: number
    remainingTokens: number
    resetTime: string
  }
  health: {
    lastCheck: string
    responseTime: number
    errorRate: number
  }
  stats: {
    totalRequests: number
    totalTokens: number
    avgResponseTime: number
    successRate: number
  }
}

export interface GatewayMetrics {
  totalRequests: number
  totalTokens: number
  avgResponseTime: number
  successRate: number
  activeConnections: number
  requestsPerSecond: number
  tokensPerSecond: number
  errorRate: number
  uptime: string
}

export interface ChatMessage {
  role: 'user' | 'assistant' | 'system'
  content: string
  timestamp: Date
}

export interface ChatCompletion {
  model: string
  messages: ChatMessage[]
  maxTokens?: number
  temperature?: number
  topP?: number
  stream?: boolean
}

export interface RoutingConfig {
  strategy: 'round-robin' | 'weighted' | 'least-latency' | 'cost-optimized' | 'random'
  providers: string[]
  weights?: Record<string, number>
  fallbackEnabled: boolean
  circuitBreakerEnabled: boolean
  retryPolicy: {
    maxRetries: number
    retryDelay: number
  }
}

export interface SystemConfig {
  server: {
    host: string
    port: number
    cors: boolean
  }
  routing: RoutingConfig
  monitoring: {
    enabled: boolean
    interval: number
    alerts: {
      errorRate: number
      responseTime: number
    }
  }
  rateLimit: {
    enabled: boolean
    requestsPerMinute: number
  }
}

export interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: {
    code: string
    message: string
  }
}