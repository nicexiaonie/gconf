.PHONY: help test test-verbose test-coverage bench fmt lint clean example run-basic run-advanced run-env

# 默认目标
help:
	@echo "Gconf - Go 配置管理工具"
	@echo ""
	@echo "可用命令:"
	@echo "  make test          - 运行测试"
	@echo "  make test-verbose  - 运行测试（详细输出）"
	@echo "  make test-coverage - 运行测试并生成覆盖率报告"
	@echo "  make bench         - 运行性能测试"
	@echo "  make fmt           - 格式化代码"
	@echo "  make lint          - 运行代码检查"
	@echo "  make clean         - 清理构建文件"
	@echo "  make example       - 运行所有示例"
	@echo "  make run-basic     - 运行基础示例"
	@echo "  make run-advanced  - 运行高级示例"
	@echo "  make run-env       - 运行环境变量示例"

# 运行测试
test:
	@echo "Running tests..."
	go test -v ./...

# 详细测试
test-verbose:
	@echo "Running tests with verbose output..."
	go test -v -race ./...

# 测试覆盖率
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

# 性能测试
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# 格式化代码
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

# 代码检查
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

# 清理
clean:
	@echo "Cleaning..."
	go clean
	rm -f coverage.txt coverage.html
	rm -f example/basic/basic
	rm -f example/advanced/advanced
	rm -f example/env/env
	rm -f test/test_gconf

# 运行所有示例
example: run-basic run-advanced run-env

# 运行基础示例
run-basic:
	@echo "Running basic example..."
	cd example/basic && go run main.go

# 运行高级示例
run-advanced:
	@echo "Running advanced example..."
	cd example/advanced && go run main.go

# 运行环境变量示例
run-env:
	@echo "Running environment variable example..."
	cd example/env && go run main.go

# 安装依赖
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# 更新依赖
update-deps:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

# 构建示例
build-examples:
	@echo "Building examples..."
	cd example/basic && go build -o basic main.go
	cd example/advanced && go build -o advanced main.go
	cd example/env && go build -o env main.go
	@echo "Examples built successfully!"

