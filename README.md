# LLM Gateway 前后端分离项目

🚀 **基于 Go + React + TypeScript 构建的高性能大语言模型API网关管理平台**

## 📋 项目概述

LLM Gateway 是一个现代化的前后端分离项目，提供统一的大语言模型API管理和可视化监控界面。

### 🏗️ 架构特点

- **后端**: Go + Gin 框架，提供高性能API服务
- **前端**: React 18 + TypeScript + Ant Design，现代化管理界面
- **状态管理**: Zustand 轻量级状态管理
- **图表可视化**: Recharts + ECharts 双重图表支持
- **开发工具**: Vite 构建，ESLint 代码规范

## 🌟 核心功能

### 后端功能
- ✅ 多LLM提供商适配（OpenAI、Claude、百度文心等）
- ✅ 智能路由和负载均衡
- ✅ 请求限流和断路器
- ✅ 实时监控和指标收集
- ✅ RESTful API 接口
- ✅ CORS 跨域支持
- ✅ 健康检查和状态监控

### 前端功能
- 🎨 现代化管理仪表板
- 📊 实时指标和性能监控
- 🔧 提供商配置管理
- 💬 AI聊天测试界面
- ⚙️ 系统设置和配置
- 📈 可视化图表和统计

## 📁 项目结构

```
LLM-Gateway-Project/
├── LLM-Gateway/                    # 后端项目
│   ├── cmd/server/                 # 服务器入口
│   ├── internal/                   # 内部模块
│   │   ├── gateway/               # 网关核心
│   │   ├── providers/             # 提供商适配器
│   │   ├── router/                # 路由管理
│   │   ├── auth/                  # 认证授权
│   │   └── middleware/            # 中间件
│   ├── pkg/                       # 公共包
│   └── docs/                      # 文档
├── llm-gateway-frontend/           # 前端项目
│   ├── src/
│   │   ├── components/            # 通用组件
│   │   ├── pages/                 # 页面组件
│   │   ├── services/              # API服务
│   │   ├── store/                 # 状态管理
│   │   ├── types/                 # 类型定义
│   │   └── utils/                 # 工具函数
│   ├── public/                    # 静态资源
│   └── package.json               # 依赖配置
├── start-project.sh               # 一键启动脚本
└── README.md                      # 项目文档
```

## 🚀 快速开始

### 环境要求

- **Go**: 1.19+
- **Node.js**: 16+
- **npm**: 8+

### 一键启动

```bash
# 克隆项目后，在项目根目录执行
./start-project.sh
```

这将自动：
1. 检查环境依赖
2. 启动后端服务 (端口 8080)
3. 安装前端依赖
4. 启动前端开发服务器 (端口 3000)

### 手动启动

#### 启动后端
```bash
cd LLM-Gateway
go mod tidy
go run cmd/server/main.go
```

#### 启动前端
```bash
cd llm-gateway-frontend
npm install
npm run dev
```

## 🖥️ 访问界面

启动成功后，您可以访问：

- **前端管理界面**: http://localhost:3000
- **后端API**: http://localhost:8080
- **健康检查**: http://localhost:8080/health
- **API文档**: http://localhost:8080/v1/admin/status

## 📊 功能展示

### 1. 仪表板 (Dashboard)
- 实时系统指标监控
- 提供商状态概览
- 性能趋势图表
- 警告和通知

### 2. 模型提供商管理 (Providers)
- 提供商配置和管理
- 状态监控和测试
- 性能指标查看
- 在线添加/编辑提供商

### 3. 指标监控 (Metrics)
- 详细的性能数据分析
- 多维度图表展示
- 实时和历史数据
- 错误率和延迟监控

### 4. 聊天测试 (Chat)
- 直接测试AI模型
- 支持多种模型切换
- 参数调节（温度、最大Token等）
- 实时对话界面

### 5. 系统设置 (Settings)
- 服务器配置管理
- 路由策略设置
- 监控和告警配置
- 限流规则设置

## 🔧 API 接口

### 核心端点

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/health` | 健康检查 |
| POST | `/v1/chat/completions` | 聊天完成（OpenAI兼容） |
| GET | `/v1/admin/status` | 系统状态 |
| GET | `/v1/admin/providers` | 获取提供商列表 |
| GET | `/v1/admin/metrics` | 获取系统指标 |
| GET | `/v1/admin/config` | 获取系统配置 |
| PUT | `/v1/admin/config` | 更新系统配置 |
| POST | `/v1/admin/providers/:id/test` | 测试提供商 |
| PUT | `/v1/admin/providers/:id` | 更新提供商 |

## 🔨 开发指南

### 后端开发
```bash
cd LLM-Gateway
go mod tidy
go run cmd/server/main.go
```

### 前端开发
```bash
cd llm-gateway-frontend
npm run dev    # 开发服务器
npm run build  # 构建生产版本
npm run lint   # 代码规范检查
```

### 构建部署
```bash
# 后端构建
cd LLM-Gateway
go build -o bin/llm-gateway cmd/server/main.go

# 前端构建
cd llm-gateway-frontend
npm run build
```

## 🛠️ 技术栈

### 后端技术
- **语言**: Go 1.19+
- **框架**: Gin Web Framework
- **日志**: Logrus
- **配置**: Viper
- **HTTP**: 标准库 net/http

### 前端技术
- **语言**: TypeScript
- **框架**: React 18
- **构建**: Vite
- **UI库**: Ant Design 5.x
- **状态**: Zustand
- **图表**: Recharts + ECharts
- **HTTP**: Axios
- **路由**: React Router DOM

## 📈 性能特点

- **高并发**: 支持 >10,000 QPS
- **低延迟**: P99 < 100ms (网关层)
- **高可用**: 99.9% 可用性保障
- **智能路由**: 多种负载均衡策略
- **故障恢复**: 断路器和重试机制

## 🔐 安全特性

- CORS 跨域配置
- API 密钥认证
- 请求限流保护
- 输入验证和清理
- 错误信息隐藏

## 📝 开发计划

- [ ] 实时WebSocket监控
- [ ] 多租户支持
- [ ] 插件扩展系统
- [ ] Docker容器化
- [ ] Kubernetes部署
- [ ] 监控告警集成

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

本项目采用 MIT 许可证。

---

**🚀 LLM Gateway - 让AI模型管理更简单！**