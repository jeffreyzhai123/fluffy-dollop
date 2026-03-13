.PHONY: help build up down logs test produce

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build all services
	docker-compose build

up: ## Start all services
	docker-compose up -d

down: ## Stop all services
	docker-compose down

logs: ## Tail logs
	docker-compose logs -f

test: ## Run integration tests
	go test ./test/integration/... -v

produce: ## Generate sample logs (requires ingestion running)
	./scripts/producer/generate-logs.sh

monitor: ## Start with monitoring stack
	docker-compose --profile monitoring up -d

clean: ## Clean up volumes and containers
	docker-compose down -v