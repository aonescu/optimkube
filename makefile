# Kubernetes Cost Optimizer Makefile

# Variables
APP_NAME = cost-optimizer
VERSION ?= latest
REGISTRY ?= localhost:5000
IMAGE = $(REGISTRY)/$(APP_NAME):$(VERSION)
NAMESPACE = kube-system

# Go variables
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED = 0

# Build variables
BUILD_DIR = build
BINARY_NAME = $(APP_NAME)

.PHONY: help build test clean docker-build docker-push deploy undeploy dev lint fmt vet deps

help: ## Display this help message
	@echo "Kubernetes Cost Optimizer - Build and Deployment"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Download Go dependencies
	@echo "📦 Downloading dependencies..."
	go mod download
	go mod tidy

fmt: ## Format Go code
	@echo "🎨 Formatting code..."
	go fmt ./...

vet: ## Run go vet
	@echo "🔍 Running go vet..."
	go vet ./...

lint: fmt vet ## Run linting tools
	@echo "✅ Linting complete"

test: ## Run tests
	@echo "🧪 Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "📊 Coverage report generated: coverage.html"

build: deps lint ## Build the binary
	@echo "🔨 Building $(BINARY_NAME)..."
	mkdir -p $(BUILD_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
		go build -ldflags="-w -s" -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "✅ Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

docker-build: ## Build Docker image
	@echo "🐳 Building Docker image: $(IMAGE)"
	docker build -t $(IMAGE) .
	docker tag $(IMAGE) $(REGISTRY)/$(APP_NAME):latest
	@echo "✅ Docker image built: $(IMAGE)"

docker-push: docker-build ## Push Docker image to registry
	@echo "📤 Pushing Docker image: $(IMAGE)"
	docker push $(IMAGE)
	docker push $(REGISTRY)/$(APP_NAME):latest
	@echo "✅ Docker image pushed"

deploy: ## Deploy to Kubernetes
	@echo "🚀 Deploying to Kubernetes..."
	kubectl apply -f k8s-manifests.yaml
	kubectl rollout status deployment/$(APP_NAME) -n $(NAMESPACE)
	@echo "✅ Deployment complete"

undeploy: ## Remove from Kubernetes
	@echo "🗑️  Removing from Kubernetes..."
	kubectl delete -f k8s-manifests.yaml --ignore-not-found=true
	@echo "✅ Undeployment complete"

restart: ## Restart the deployment
	@echo "🔄 Restarting deployment..."
	kubectl rollout restart deployment/$(APP_NAME) -n $(NAMESPACE)
	kubectl rollout status deployment/$(APP_NAME) -n $(NAMESPACE)
	@echo "✅ Restart complete"

logs: ## Show application logs
	@echo "📜 Showing logs..."
	kubectl logs -f deployment/$(APP_NAME) -n $(NAMESPACE)

status: ## Show deployment status
	@echo "📊 Deployment Status:"
	kubectl get deployment $(APP_NAME) -n $(NAMESPACE)
	@echo ""
	@echo "📊 Pod Status:"
	kubectl get pods -l app=$(APP_NAME) -n $(NAMESPACE)
	@echo ""
	@echo "📊 Service Status:"
	kubectl get service $(APP_NAME) -n $(NAMESPACE)

port-forward: ## Port forward to local machine
	@echo "🔌 Port forwarding to localhost:8080..."
	kubectl port-forward service/$(APP_NAME) 8080:80 -n $(NAMESPACE)

dev: build ## Run locally for development
	@echo "🛠️  Starting development server..."
	./$(BUILD_DIR)/$(BINARY_NAME)

clean: ## Clean build artifacts
	@echo "🧹 Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	docker rmi $(IMAGE) $(REGISTRY)/$(APP_NAME):latest 2>/dev/null || true
	@echo "✅ Clean complete"

check-cluster: ## Check if cluster is accessible
	@echo "🔍 Checking cluster access..."
	kubectl cluster-info
	kubectl get nodes
	@echo "✅ Cluster is accessible"

check-metrics: ## Check if metrics server is running
	@echo "🔍 Checking metrics server..."
	kubectl get pods -n kube-system | grep metrics-server || echo "❌ Metrics server not found"
	kubectl top nodes 2>/dev/null && echo "✅ Metrics server is working" || echo "❌ Metrics server not responding"

install-metrics: ## Install metrics server (for development)
	@echo "📦 Installing metrics server..."
	kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
	@echo "⏳ Waiting for metrics server to be ready..."
	kubectl wait --for=condition=ready pod -l k8s-app=metrics-server -n kube-system --timeout=60s
	@echo "✅ Metrics server installed"

setup: check-cluster install-metrics deploy ## Full setup (cluster check, metrics server, deploy)
	@echo "🎉 Setup complete! Use 'make port-forward' to access the dashboard"

quick-deploy: docker-build deploy ## Quick build and deploy
	@echo "⚡ Quick deployment complete"

dashboard: port-forward ## Open dashboard in browser
	@echo "🌐 Opening dashboard..."
	@sleep 2
	@which xdg-open >/dev/null && xdg-open http://localhost:8080 || \
	 which open >/dev/null && open http://localhost:8080 || \
	 echo "📱 Dashboard available at: http://localhost:8080"

ci-test: deps lint test ## Run CI tests
	@echo "🤖 CI tests complete"

ci-build: ci-test docker-build ## Full CI build
	@echo "🤖 CI build complete"

release: clean ci-build docker-push ## Create a release
	@echo "🎊 Release $(VERSION) complete"

# Development helpers
watch: ## Watch for changes and rebuild
	@echo "👀 Watching for changes..."
	@which fswatch >/dev/null || (echo "❌ fswatch not installed. Install with: brew install fswatch" && exit 1)
	fswatch -o . | xargs -n1 -I{} make build

install-tools: ## Install development tools
	@echo "🔧 Installing development tools..."
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "✅ Development tools installed"

# Monitoring and debugging
describe: ## Describe the deployment
	kubectl describe deployment $(APP_NAME) -n $(NAMESPACE)

events: ## Show recent events
	kubectl get events -n $(NAMESPACE) --sort-by='.lastTimestamp'

shell: ## Get shell in running pod
	@POD=$$(kubectl get pods -l app=$(APP_NAME) -n $(NAMESPACE) -o jsonpath="{.items[0].metadata.name}"); \
	echo "🐚 Opening shell in pod: $$POD"; \
	kubectl exec -it $$POD -n $(NAMESPACE) -- /bin/sh

# Configuration management
config-show: ## Show current configuration
	kubectl get configmap $(APP_NAME)-config -n $(NAMESPACE) -o yaml

config-edit: ## Edit configuration
	kubectl edit configmap $(APP_NAME)-config -n $(NAMESPACE)

# Backup and restore
backup: ## Backup current configuration
	@echo "💾 Backing up configuration..."
	mkdir -p backup
	kubectl get configmap $(APP_NAME)-config -n $(NAMESPACE) -o yaml > backup/config-$$(date +%Y%m%d-%H%M%S).yaml
	@echo "✅ Configuration backed up"

# Performance testing
load-test: ## Run simple load test
	@echo "⚡ Running load test..."
	@which ab >/dev/null || (echo "❌ Apache Bench (ab) not installed" && exit 1)
	ab -n 100 -c 10 http://localhost:8080/api/cost-summary

# Documentation
docs: ## Generate documentation
	@echo "📚 Generating documentation..."
	go doc -all . > docs/api.md
	@echo "✅ Documentation generated"

# All-in-one commands
dev-setup: install-tools setup port-forward ## Complete development setup

production-deploy: ci-build docker-push deploy ## Production deployment

.DEFAULT_GOAL := help