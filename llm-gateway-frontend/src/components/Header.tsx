import React from 'react'
import { Layout, Avatar, Dropdown, Space, Badge, Button, Typography } from 'antd'
import { 
  UserOutlined, 
  SettingOutlined, 
  LogoutOutlined,
  BellOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined
} from '@ant-design/icons'
import { useStore } from '../store/useStore'

const { Header: AntHeader } = Layout
const { Text } = Typography

const Header: React.FC = () => {
  const { sidebarCollapsed, setSidebarCollapsed } = useStore()

  const userMenuItems = [
    {
      key: 'settings',
      icon: <SettingOutlined />,
      label: '系统设置',
    },
    {
      type: 'divider' as const,
    },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
    },
  ]

  const handleMenuClick = ({ key }: { key: string }) => {
    switch (key) {
      case 'settings':
        // 跳转到设置页面
        break
      case 'logout':
        // 退出登录
        localStorage.removeItem('auth_token')
        window.location.reload()
        break
    }
  }

  return (
    <AntHeader 
      style={{ 
        padding: '0 24px',
        background: '#fff',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        borderBottom: '1px solid #f0f0f0',
        marginLeft: sidebarCollapsed ? '80px' : '240px',
        transition: 'margin-left 0.2s'
      }}
    >
      <Space>
        <Button
          type="text"
          icon={sidebarCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
          style={{ fontSize: '16px', width: 32, height: 32 }}
        />
        <Text strong style={{ fontSize: '18px' }}>
          LLM Gateway 管理平台
        </Text>
      </Space>

      <Space>
        <Badge count={3} size="small">
          <Button type="text" icon={<BellOutlined />} />
        </Badge>
        
        <Dropdown
          menu={{ items: userMenuItems, onClick: handleMenuClick }}
          placement="bottomRight"
        >
          <Space style={{ cursor: 'pointer' }}>
            <Avatar icon={<UserOutlined />} />
            <Text>管理员</Text>
          </Space>
        </Dropdown>
      </Space>
    </AntHeader>
  )
}

export default Header