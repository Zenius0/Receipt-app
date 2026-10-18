# Yerel geliştirme komutları.
#   make help    → tüm komutlar
#
# Tüm komutlar proje kökünden çalıştırılır.

SHELL := /bin/bash
BACKEND := backend
DB_SERVICE := postgres

.DEFAULT_GOAL := help

.PHONY: help
help: ## Bu yardım metnini göster
	@echo ""
	@echo "  Fatura — yerel geliştirme komutları"
	@echo "  ─────────────────────────────────────"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
	@echo ""

# --- Kurulum -----------------------------------------------------------

.PHONY: env
env: ## backend/.env dosyasını örnekten üret (JWT_SECRET otomatik)
	@if [ -f $(BACKEND)/.env ]; then \
		echo "  $(BACKEND)/.env zaten var, dokunulmadı"; \
	else \
		sed -e "s|^JWT_SECRET=.*|JWT_SECRET=$$(openssl rand -hex 32)|" \
			$(BACKEND)/.env.example > $(BACKEND)/.env; \
		echo "  $(BACKEND)/.env oluşturuldu (JWT_SECRET üretildi)"; \
		echo "  ⚠ GEMINI_API_KEY ve GOOGLE_CLIENT_IDS değerlerini elle doldurun"; \
	fi

.PHONY: setup
setup: env db-up ## Sıfırdan kur: .env + veritabanı
	@echo ""
	@echo "  Hazır. Şimdi:"
	@echo "    1) $(BACKEND)/.env içindeki GEMINI_API_KEY ve GOOGLE_CLIENT_IDS'i doldurun"
	@echo "    2) make run"

# --- Veritabanı --------------------------------------------------------

.PHONY: db-up
db-up: ## PostgreSQL'i başlat (Docker)
	@docker compose up -d $(DB_SERVICE)
	@echo "  PostgreSQL hazırlanıyor..."
	@until [ "$$(docker compose ps -q $(DB_SERVICE) | xargs -r docker inspect -f '{{.State.Health.Status}}' 2>/dev/null)" = "healthy" ]; do sleep 1; done
	@echo "  ✓ PostgreSQL hazır → localhost:5433/fatura"

.PHONY: db-down
db-down: ## PostgreSQL'i durdur (veriler korunur)
	@docker compose stop $(DB_SERVICE)

.PHONY: db-reset
db-reset: ## PostgreSQL'i durdur ve VERİYİ SİL, sıfırdan kur
	@read -p "  Tüm yerel veriler silinecek. Emin misiniz? [y/N] " a; \
	[ "$$a" = "y" ] || { echo "  iptal edildi"; exit 1; }
	@docker compose down -v
	@docker compose up -d $(DB_SERVICE)
	@echo "  ✓ Veritabanı sıfırlandı"

.PHONY: db-shell
db-shell: ## psql ile veritabanına gir
	@docker compose exec $(DB_SERVICE) psql -U fatura -d fatura

.PHONY: db-logs
db-logs: ## PostgreSQL loglarını izle
	@docker compose logs -f $(DB_SERVICE)

# --- Backend -----------------------------------------------------------

.PHONY: run
run: env ## API'yi localhost:8080'de çalıştır (migration otomatik)
	@cd $(BACKEND) && set -a && source .env && set +a && go run ./cmd/api

.PHONY: build
build: ## Production binary derle
	@cd $(BACKEND) && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api
	@echo "  ✓ $(BACKEND)/bin/api"

.PHONY: test
test: ## Tüm testleri çalıştır
	@cd $(BACKEND) && go test ./...

.PHONY: test-postgres
test-postgres: env ## Gerçek PostgreSQL'e karşı uçtan uca test
	@cd $(BACKEND) && set -a && source .env && set +a && \
		DATABASE_URL="postgres://fatura:fatura@localhost:5433/fatura?sslmode=disable" \
		go test ./internal/httpapi/handlers -run Postgres -v

.PHONY: vet
vet: ## go vet + gofmt kontrolü
	@cd $(BACKEND) && go vet ./...
	@cd $(BACKEND) && test -z "$$(gofmt -l .)" || { echo "gofmt gerekli:"; gofmt -l .; exit 1; }
	@echo "  ✓ vet + gofmt temiz"

.PHONY: fmt
fmt: ## Kodu biçimlendir
	@cd $(BACKEND) && gofmt -w .

.PHONY: check
check: vet test ## CI kontrolü: vet + gofmt + test

.PHONY: clean
clean: ## Derleme çıktılarını temizle
	@cd $(BACKEND) && rm -rf bin
	@echo "  ✓ temizlendi"