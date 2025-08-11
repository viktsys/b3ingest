.PHONY: help setup build clean deps test run-ingest run-server docker-build docker-up docker-down docker-logs status

# Default target
help:
	@echo "B3 Trading Data Ingestion Tool - Makefile"
	@echo ""
	@echo "Available commands:"
	@echo "  setup           - Complete setup: build Docker containers, start services, and build binary"
	@echo "  build           - Build the Go binary"
	@echo "  deps            - Download Go dependencies"
	@echo "  test            - Run tests"
	@echo "  clean           - Clean build artifacts"
	@echo ""
	@echo "Docker commands:"
	@echo "  docker-build    - Build Docker containers"
	@echo "  docker-up       - Start services with Docker Compose"
	@echo "  docker-down     - Stop and remove containers"
	@echo "  docker-logs     - Show container logs"
	@echo "  status          - Show container status"
	@echo ""
	@echo "Application commands:"
	@echo "  run-ingest      - Run data ingestion (requires DATA_DIR env var or uses data/)"
	@echo "  run-server      - Run API server"
	@echo ""
	@echo "Example usage:"
	@echo "  make setup                    # Complete project setup"
	@echo "  make run-ingest DATA_DIR=data # Ingest data from data/ directory"
	@echo "  make run-server              # Start API server"

# Complete setup command
setup: docker-build docker-up build
	@echo "✅ Setup completed successfully!"
	@echo "💡 Next steps:"
	@echo "   1. Place your CSV/TXT files in the data/ directory"
	@echo "   2. Run: make run-ingest"
	@echo "   3. Test the API: curl \"http://localhost:8080/api/trades/stats?ticker=PETR4\""

# Build Go binary
build:
	@echo "🔨 Building Go binary..."
	@mkdir -p bin
	go build -o bin/b3ingest .
	@echo "✅ Binary built successfully at bin/b3ingest"

# Download Go dependencies
deps:
	@echo "📦 Downloading Go dependencies..."
	go mod download
	go mod tidy
	@echo "✅ Dependencies updated"

# Run tests
test:
	@echo "🧪 Running tests..."
	go test -v ./...

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf bin/
	docker compose down --volumes --remove-orphans
	@echo "✅ Cleanup completed"

# Docker commands
docker-build:
	@echo "🐳 Building Docker containers..."
	docker compose build
	@echo "✅ Docker containers built"

docker-up:
	@echo "🚀 Starting services with Docker Compose..."
	docker compose up -d
	@echo "⏳ Waiting for services to be ready..."
	@sleep 5
	@echo "✅ Services started successfully"

docker-down:
	@echo "🛑 Stopping Docker services..."
	docker compose down
	@echo "✅ Services stopped"

docker-logs:
	@echo "📋 Showing container logs..."
	docker compose logs -f

status:
	@echo "📊 Container status:"
	docker compose ps

# Application commands
run-ingest: build
	@echo "📥 Starting data ingestion..."
	@if [ -z "$(DATA_DIR)" ]; then \
		echo "Using default data directory: data/"; \
		./bin/b3ingest ingest data/; \
	else \
		echo "Using data directory: $(DATA_DIR)"; \
		./bin/b3ingest ingest $(DATA_DIR); \
	fi

run-server: build
	@echo "🌐 Starting API server..."
	./bin/b3ingest server

# Development helpers
dev-setup: setup
	@echo "🔧 Development environment setup completed"
	@echo "💡 Development tips:"
	@echo "   - Use 'make docker-logs' to monitor container logs"
	@echo "   - Use 'make status' to check container status"
	@echo "   - Use 'make test' to run tests"
	@echo "   - Use 'make clean' to reset everything"

# Database helpers
db-reset: docker-down
	@echo "🔄 Resetting database..."
	docker volume rm b3ingest_postgres_data 2>/dev/null || true
	$(MAKE) docker-up
	@echo "✅ Database reset completed"
