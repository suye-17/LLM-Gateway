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
  // 基础指标
  totalRequests: number
  totalTokens: number
  avgResponseTime: number
  successRate: number
  activeConnections: number
  requestsPerSecond: number
  tokensPerSecond: number
  errorRate: number
  uptime: string
  
  // 智能路由指标 (Week4)
  requests_total?: number
  requests_success?: number
  requests_failed?: number
  avg_latency_ms?: number
  providers_healthy?: number
  smart_router?: {
    strategy: string
    requests: number
    providers: string[]
    health_checks: {
      enabled: boolean
      interval: string
    }
    circuit_breaker: {
      enabled: boolean
      threshold: number
      timeout: string
    }
    load_balancing: {
      algorithms: string[]
      current: string
    }
    metrics_endpoint: string
  }
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
  strategy: 'round_robin' | 'weighted_round_robin' | 'least_connections' | 'least_latency' | 'health_based' | 'cost_optimized' | 'random'
  providers: string[]
  weights?: Record<string, number>
  failoverEnabled: boolean
  circuitBreakerEnabled: boolean
  retryPolicy: {
    maxRetries: number
    retryDelay: number
  }
  healthCheckInterval?: string
  metricsEnabled?: boolean
}

// 智能路由器状态接口
export interface SmartRouterStatus {
  strategy: string
  requests: number
  providers: string[]
  health_checks: {
    enabled: boolean
    interval: string
  }
  circuit_breaker: {
    enabled: boolean
    threshold: number
    timeout: string
  }
  load_balancing: {
    algorithms: string[]
    current: string
  }
  metrics_endpoint: string
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