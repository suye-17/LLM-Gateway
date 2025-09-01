import React from 'react'
import { Layout, Menu } from 'antd'
import { useNavigate, useLocation } from 'react-router-dom'
import {
  DashboardOutlined,
  ApiOutlined,
  BarChartOutlined,
  MessageOutlined,
  SettingOutlined,
  ThunderboltOutlined
} from '@ant-design/icons'
import { useStore } from '../store/useStore'

const { Sider } = Layout

const Sidebar: React.FC = () => {
  const navigate = useNavigate()
  const location = useLocation()
  const { sidebarCollapsed } = useStore()

  const menuItems = [
    {
      key: '/',
      icon: <DashboardOutlined />,
      label: '仪表板',
    },
    {
      key: '/providers',
      icon: <ApiOutlined />,
      label: '模型提供商',
    },
    {
      key: '/metrics',
      icon: <BarChartOutlined />,
      label: '指标监控',
    },
    {
      key: '/chat',
      icon: <MessageOutlined />,
      label: '聊天测试',
    },
    {
      key: '/settings',
      icon: <SettingOutlined />,
      label: '系统设置',
    },
  ]

  const handleMenuClick = ({ key }: { key: string }) => {
    navigate(key)
  }

  return (
    <Sider
      collapsible
      collapsed={sidebarCollapsed}
      trigger={null}
      theme="dark"
      width={240}
      style={{
        overflow: 'auto',
        height: '100vh',
        position: 'fixed',
        left: 0,
        top: 0,
        zIndex: 200,
      }}
    >
      <div 
        style={{ 
          height: '64px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#fff',
          fontSize: '18px',
          fontWeight: 'bold',
          borderBottom: '1px solid #303030'
        }}
      >
        <ThunderboltOutlined style={{ marginRight: sidebarCollapsed ? 0 : 8 }} />
        {!sidebarCollapsed && 'LLM Gateway'}
      </div>
      
      <Menu
        theme="dark"
        mode="inline"
        selectedKeys={[location.pathname]}
        items={menuItems}
        onClick={handleMenuClick}
        style={{ borderRight: 0, paddingTop: 16 }}
      />
    </Sider>
  )
}

export default Sidebar