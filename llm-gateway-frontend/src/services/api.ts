import axios, { AxiosInstance, AxiosResponse } from 'axios'
import { message } from 'antd'
import { ApiResponse, Provider, GatewayMetrics, ChatCompletion, SystemConfig } from '../types'

class ApiService {
  private api: AxiosInstance

  constructor() {
    this.api = axios.create({
      baseURL: '/api',
      timeout: 10000,
      headers: {
        'Content-Type': 'application/json',
      }
    })

    // 请求拦截器
    this.api.interceptors.request.use(
      (config) => {
        // 可以在这里添加认证token
        const token = localStorage.getItem('auth_token')
        if (token) {
          config.headers.Authorization = `Bearer ${token}`
        }
        return config
      },
      (error) => {
        return Promise.reject(error)
      }
    )

    // 响应拦截器
    this.api.interceptors.response.use(
      (response: AxiosResponse) => {
        return response
      },
      (error) => {
        const errorMessage = error.response?.data?.error?.message || '请求失败'
        message.error(errorMessage)
        return Promise.reject(error)
      }
    )
  }

  // 健康检查
  async healthCheck(): Promise<boolean> {
    try {
      await this.api.get('/health')
      return true
    } catch {
      return false
    }
  }

  // 获取提供商列表
  async getProviders(): Promise<Provider[]> {
    try {
      const response = await this.api.get<ApiResponse<Provider[]>>('/admin/providers')
      return response.data.data || []
    } catch (error) {
      console.log('Providers endpoint might not return wrapped data, returning empty array')
      return []
    }
  }

  // 获取系统指标
  async getMetrics(): Promise<GatewayMetrics> {
    const response = await this.api.get<GatewayMetrics>('/admin/metrics')
    return response.data
  }

  // 获取系统状态
  async getSystemStatus(): Promise<any> {
    const response = await this.api.get<ApiResponse<any>>('/admin/status')
    return response.data.data
  }

  // 聊天完成请求
  async chatCompletion(request: ChatCompletion): Promise<any> {
    const response = await this.api.post('/chat/completions', request)
    return response.data
  }

  // 获取系统配置
  async getSystemConfig(): Promise<SystemConfig> {
    const response = await this.api.get<ApiResponse<SystemConfig>>('/admin/config')
    return response.data.data!
  }

  // 更新系统配置
  async updateSystemConfig(config: Partial<SystemConfig>): Promise<void> {
    await this.api.put('/admin/config', config)
    message.success('配置更新成功')
  }

  // 测试提供商连接
  async testProvider(providerId: string): Promise<boolean> {
    try {
      await this.api.post(`/admin/providers/${providerId}/test`)
      return true
    } catch {
      return false
    }
  }

  // 更新提供商配置
  async updateProvider(providerId: string, config: Partial<Provider>): Promise<void> {
    await this.api.put(`/admin/providers/${providerId}`, config)
    message.success('提供商配置更新成功')
  }
}

export const apiService = new ApiService()
export default apiService