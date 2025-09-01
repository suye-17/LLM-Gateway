import React from 'react'
import { Routes, Route } from 'react-router-dom'
import { Layout } from 'antd'
import Sidebar from './components/Sidebar'
import Header from './components/Header'
import Dashboard from './pages/Dashboard'
import Providers from './pages/Providers'
import Metrics from './pages/Metrics'
import Settings from './pages/Settings'
import Chat from './pages/Chat'
import { useStore } from './store/useStore'

const { Content } = Layout

function App() {
  const { sidebarCollapsed } = useStore()
  return (
    <Layout>
      <Sidebar />
      <Layout>
        <Header />
        <Content 
          style={{ 
            margin: '16px', 
            marginLeft: sidebarCollapsed ? '96px' : '256px', 
            overflow: 'auto',
            transition: 'margin-left 0.2s',
            minHeight: 'calc(100vh - 64px)'
          }}
        >
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/providers" element={<Providers />} />
            <Route path="/metrics" element={<Metrics />} />
            <Route path="/chat" element={<Chat />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </Content>
      </Layout>
    </Layout>
  )
}

export default App