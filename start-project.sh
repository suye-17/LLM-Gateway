#!/bin/bash

# LLM Gateway 前后端分离项目启动脚本
echo "🚀 启动 LLM Gateway 前后端分离项目..."

# 检查依赖
echo "📋 检查依赖..."

# 检查 Go
if ! command -v go &> /dev/null; then
    echo "❌ Go 未安装，请先安装 Go"
    exit 1
fi

# 检查 Node.js
if ! command -v node &> /dev/null; then
    echo "❌ Node.js 未安装，请先安装 Node.js"
    exit 1
fi

# 检查 npm
if ! command -v npm &> /dev/null; then
    echo "❌ npm 未安装，请先安装 npm"
    exit 1
fi

echo "✅ 依赖检查完成"

# 启动后端服务
echo "🔧 启动后端服务..."
cd LLM-Gateway
go mod tidy
echo "后端服务正在启动，端口: 8080"
go run cmd/server/main.go &
BACKEND_PID=$!
echo "后端服务 PID: $BACKEND_PID"

# 等待后端启动
echo "⏳ 等待后端服务启动..."
sleep 3

# 检查后端健康状态
echo "🩺 检查后端健康状态..."
if curl -s http://localhost:8080/health > /dev/null; then
    echo "✅ 后端服务启动成功"
else
    echo "❌ 后端服务启动失败"
    kill $BACKEND_PID
    exit 1
fi

# 启动前端服务
echo "🎨 启动前端服务..."
cd ../llm-gateway-frontend

# 安装依赖（如果 node_modules 不存在）
if [ ! -d "node_modules" ]; then
    echo "📦 安装前端依赖..."
    npm install
fi

echo "前端服务正在启动，端口: 3000"
npm run dev &
FRONTEND_PID=$!
echo "前端服务 PID: $FRONTEND_PID"

# 等待前端启动
echo "⏳ 等待前端服务启动..."
sleep 5

echo ""
echo "🎉 项目启动完成！"
echo "📊 后端API: http://localhost:8080"
echo "🖥️  前端界面: http://localhost:3000"
echo "📚 健康检查: http://localhost:8080/health"
echo ""
echo "💡 服务进程:"
echo "   - 后端 PID: $BACKEND_PID"
echo "   - 前端 PID: $FRONTEND_PID"
echo ""
echo "⚡ 按 Ctrl+C 停止所有服务"

# 监听中断信号，优雅关闭服务
trap "echo ''; echo '🛑 正在停止服务...'; kill $BACKEND_PID $FRONTEND_PID 2>/dev/null; echo '✅ 所有服务已停止'; exit 0" INT

# 保持脚本运行
wait