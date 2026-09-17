.PHONY: build build-go test lint vet clean run frontend sync-dist smoke

CGO_ENABLED ?= 0
export CGO_ENABLED

VERSION_FILE := VERSION
VERSION ?= $(shell cat $(VERSION_FILE) 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# Full binary: refresh embed if frontend sources are newer than pkg/web/dist,
# then go build. Directory mtime via find — see sync-frontend-if-needed.
build: sync-frontend-if-needed build-go

build-go:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/factura ./cmd/factura

# Rebuild + sync when frontend/ is newer than the committed embed, or embed
# has no index.html yet (placeholder-only).
sync-frontend-if-needed:
	@if [ ! -d frontend ]; then exit 0; fi
	@if [ ! -f pkg/web/dist/index.html ]; then \
	  echo "-> pkg/web/dist missing index.html — building frontend"; \
	  $(MAKE) frontend; \
	  exit 0; \
	fi
	@newer=$$(find frontend/src frontend/package.json frontend/index.html \
	  -type f -newer pkg/web/dist/index.html 2>/dev/null | head -1); \
	if [ -n "$$newer" ]; then \
	  echo "-> frontend newer than embed ($$newer) — rebuilding"; \
	  $(MAKE) frontend; \
	fi

frontend:
	cd frontend && npm ci && npm run build
	$(MAKE) sync-dist

sync-dist:
	mkdir -p pkg/web/dist
	rm -rf pkg/web/dist/*
	cp -a frontend/dist/. pkg/web/dist/

test:
	CGO_ENABLED=0 go test ./...
	@if [ -d frontend/node_modules ] || [ -f frontend/package-lock.json ]; then \
	  cd frontend && npm test; \
	else \
	  echo "skip frontend tests (no node_modules — run: cd frontend && npm ci)"; \
	fi

vet:
	CGO_ENABLED=0 go vet ./...

lint: vet
	@echo "vet ok"

run: build
	./bin/factura serve --addr $${FACTURA_ADDR:-:8080}

# End-to-end smoke: build, serve, curl the product surface.
smoke: build
	@tmpdir=$$(mktemp -d); \
	db="$$tmpdir/smoke.db"; \
	./bin/factura serve --addr 127.0.0.1:18081 --db "$$db" >"$$tmpdir/serve.log" 2>&1 & \
	pid=$$!; \
	trap 'kill $$pid 2>/dev/null; rm -rf "$$tmpdir"' EXIT; \
	ok=0; \
	for i in 1 2 3 4 5 6 7 8 9 10; do \
	  if curl -sf -o /dev/null -w "%{http_code}" http://127.0.0.1:18081/health | grep -q 200; then ok=1; break; fi; \
	  sleep 0.3; \
	done; \
	if [ "$$ok" != 1 ]; then echo "smoke: health not ready"; cat "$$tmpdir/serve.log"; exit 1; fi; \
	curl -sf http://127.0.0.1:18081/health | tee /tmp/factura-smoke-health.json | grep -q '"status":"ok"' || { echo "health missing status"; exit 1; }; \
	grep -q '"version"' /tmp/factura-smoke-health.json || { echo "health missing version"; exit 1; }; \
	grep -q '"plan"' /tmp/factura-smoke-health.json || { echo "health missing plan"; exit 1; }; \
	code=$$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:18081/); \
	test "$$code" = "302" || { echo "GET / want 302 got $$code"; exit 1; }; \
	code=$$(curl -s -o /tmp/factura-smoke-app.html -w "%{http_code}" http://127.0.0.1:18081/app/); \
	test "$$code" = "200" || { echo "GET /app/ want 200 got $$code"; exit 1; }; \
	grep -q 'id="root"' /tmp/factura-smoke-app.html || { echo "/app/ missing #root"; exit 1; }; \
	code=$$(curl -s -o /tmp/factura-smoke-list.json -w "%{http_code}" http://127.0.0.1:18081/invoices); \
	test "$$code" = "200" || { echo "GET /invoices want 200 got $$code"; exit 1; }; \
	grep -q '"rows"' /tmp/factura-smoke-list.json || { echo "list missing rows"; exit 1; }; \
	echo "smoke ok (version=$(VERSION))"

clean:
	rm -rf bin/
