#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Configuration
PROJECT_NAME="llm-gateway"
DEPLOY_ENV=${1:-production}
DEPLOY_DIR="/opt/${PROJECT_NAME}"

print_status "Starting deployment of LLM Gateway with Smart Router..."
print_status "Environment: ${DEPLOY_ENV}"

# Check if running as root (for production deployment)
if [[ $EUID -eq 0 ]] && [[ "$DEPLOY_ENV" == "production" ]]; then
    print_warning "Running as root for production deployment"
fi

# Check dependencies
print_status "Checking dependencies..."

# Check Docker
if ! command -v docker &> /dev/null; then
    print_error "Docker is not installed. Please install Docker first."
    exit 1
fi

# Check Docker Compose
if ! command -v docker-compose &> /dev/null; then
    print_error "Docker Compose is not installed. Please install Docker Compose first."
    exit 1
fi

print_success "Dependencies check passed"

# Create deployment directory (for production)
if [[ "$DEPLOY_ENV" == "production" ]]; then
    print_status "Creating deployment directory: ${DEPLOY_DIR}"
    sudo mkdir -p ${DEPLOY_DIR}
    sudo chown -R $USER:$USER ${DEPLOY_DIR}
fi

# Copy environment file
print_status "Setting up environment configuration..."
if [[ ! -f .env ]]; then
    if [[ -f .env.example ]]; then
        cp .env.example .env
        print_warning "Created .env from .env.example. Please update the values before continuing."
        print_warning "Press Enter after updating .env file to continue..."
        read
    else
        print_error ".env file not found and .env.example doesn't exist"
        exit 1
    fi
fi

# Validate environment file
print_status "Validating environment configuration..."
source .env

required_vars=("DB_PASSWORD" "JWT_SECRET" "OPENAI_API_KEY")
for var in "${required_vars[@]}"; do
    if [[ -z "${!var}" ]]; then
        print_error "Required environment variable $var is not set"
        exit 1
    fi
done

print_success "Environment validation passed"

# Build application
print_status "Building application..."
cd deployments
docker-compose build --no-cache

if [[ $? -ne 0 ]]; then
    print_error "Build failed"
    exit 1
fi

print_success "Build completed successfully"

# Stop existing services
print_status "Stopping existing services..."
docker-compose down --remove-orphans

# Start services
print_status "Starting services..."
docker-compose up -d

if [[ $? -ne 0 ]]; then
    print_error "Failed to start services"
    exit 1
fi

# Wait for services to be healthy
print_status "Waiting for services to be healthy..."
sleep 10

# Check service health
print_status "Checking service health..."

services=("postgres" "redis" "gateway")
for service in "${services[@]}"; do
    print_status "Checking $service..."
    
    for i in {1..30}; do
        if docker-compose ps $service | grep -q "(healthy)"; then
            print_success "$service is healthy"
            break
        elif docker-compose ps $service | grep -q "(unhealthy)"; then
            print_error "$service is unhealthy"
            docker-compose logs $service
            exit 1
        else
            print_status "Waiting for $service to be ready... ($i/30)"
            sleep 2
        fi
    done
done

# Test API endpoints
print_status "Testing API endpoints..."

# Health check
if curl -f -s http://localhost:8080/health > /dev/null; then
    print_success "Health endpoint is responding"
else
    print_error "Health endpoint is not responding"
    docker-compose logs gateway
    exit 1
fi

# Metrics endpoint
if curl -f -s http://localhost:9090/metrics > /dev/null; then
    print_success "Metrics endpoint is responding"
else
    print_warning "Metrics endpoint is not responding"
fi

# Show running services
print_status "Current service status:"
docker-compose ps

# Show useful information
print_success "Deployment completed successfully!"
echo ""
print_status "Service URLs:"
echo "  🌐 API Gateway:     http://localhost:8080"
echo "  📊 Metrics:         http://localhost:9090/metrics"
echo "  📈 Prometheus:      http://localhost:9091"
echo "  📋 Grafana:         http://localhost:3000 (admin/admin)"
echo "  🐘 PostgreSQL:      localhost:5432"
echo "  🔴 Redis:           localhost:6379"
echo ""
print_status "Smart Router Features:"
echo "  ✅ Round Robin Load Balancing"
echo "  ✅ Weighted Round Robin"
echo "  ✅ Least Connections"
echo "  ✅ Health-based Routing"
echo "  ✅ Circuit Breaker Protection"
echo "  ✅ Real-time Health Monitoring"
echo "  ✅ Comprehensive Metrics Collection"
echo ""
print_status "Useful commands:"
echo "  📜 View logs:        docker-compose logs -f gateway"
echo "  🔄 Restart gateway:  docker-compose restart gateway"
echo "  🛑 Stop all:         docker-compose down"
echo "  📊 View metrics:     curl http://localhost:9090/metrics"
echo "  ❤️  Health check:     curl http://localhost:8080/health"

# Save deployment info
cat > deployment-info.txt << EOF
LLM Gateway Deployment Information
==================================
Deployment Time: $(date)
Environment: ${DEPLOY_ENV}
Version: $(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

Service URLs:
- API Gateway: http://localhost:8080
- Metrics: http://localhost:9090/metrics  
- Prometheus: http://localhost:9091
- Grafana: http://localhost:3000

Configuration:
- Smart Router Strategy: ${SMART_ROUTER_STRATEGY:-round_robin}
- Health Check Interval: ${SMART_ROUTER_HEALTH_CHECK_INTERVAL:-30s}
- Circuit Breaker: ${SMART_ROUTER_CIRCUIT_BREAKER_ENABLED:-true}
- Metrics Enabled: ${SMART_ROUTER_METRICS_ENABLED:-true}

To view logs: docker-compose logs -f gateway
To restart: docker-compose restart gateway
To stop: docker-compose down
EOF

print_success "Deployment information saved to deployment-info.txt"