import { create } from 'zustand'
import { Provider, GatewayMetrics, SystemConfig } from '../types'
import apiService from '../services/api'

interface AppState {
  // 数据状态
  providers: Provider[]
  metrics: GatewayMetrics | null
  systemConfig: SystemConfig | null
  isLoading: boolean
  
  // UI 状态
  selectedProvider: string | null
  sidebarCollapsed: boolean
  
  // Actions
  setProviders: (providers: Provider[]) => void
  setMetrics: (metrics: GatewayMetrics) => void
  setSystemConfig: (config: SystemConfig) => void
  setSelectedProvider: (id: string | null) => void
  setSidebarCollapsed: (collapsed: boolean) => void
  setLoading: (loading: boolean) => void
  
  // 异步 Actions
  fetchProviders: () => Promise<void>
  fetchMetrics: () => Promise<void>
  fetchSystemConfig: () => Promise<void>
  refreshAll: () => Promise<void>
}

export const useStore = create<AppState>((set, get) => ({
  // 初始状态
  providers: [],
  metrics: null,
  systemConfig: null,
  isLoading: false,
  selectedProvider: null,
  sidebarCollapsed: false,

  // Setters
  setProviders: (providers) => set({ providers }),
  setMetrics: (metrics) => set({ metrics }),
  setSystemConfig: (systemConfig) => set({ systemConfig }),
  setSelectedProvider: (selectedProvider) => set({ selectedProvider }),
  setSidebarCollapsed: (sidebarCollapsed) => set({ sidebarCollapsed }),
  setLoading: (isLoading) => set({ isLoading }),

  // 异步操作
  fetchProviders: async () => {
    try {
      set({ isLoading: true })
      const providers = await apiService.getProviders()
      set({ providers })
    } catch (error) {
      console.error('Failed to fetch providers:', error)
    } finally {
      set({ isLoading: false })
    }
  },

  fetchMetrics: async () => {
    try {
      const metrics = await apiService.getMetrics()
      set({ metrics })
    } catch (error) {
      console.error('Failed to fetch metrics:', error)
    }
  },

  fetchSystemConfig: async () => {
    try {
      const systemConfig = await apiService.getSystemConfig()
      set({ systemConfig })
    } catch (error) {
      console.error('Failed to fetch system config:', error)
    }
  },

  refreshAll: async () => {
    const { fetchProviders, fetchMetrics, fetchSystemConfig } = get()
    await Promise.all([
      fetchProviders(),
      fetchMetrics(),
      fetchSystemConfig()
    ])
  }
}))