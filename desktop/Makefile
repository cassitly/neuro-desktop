# ============================================================
# desktop/Makefile — Linux and macOS. On Windows use scripts\build-all.ps1.
# The shipped product is the Go server, the Python agent and the dashboard.
# ============================================================

GO ?= go
PYTHON ?= python3

all: build

build: server frontend

server:
	cd apps/neuro-integration && $(GO) build -o dist/neuro-integration .

frontend:
	cd frontend && (test -f package-lock.json && npm ci || npm install) && npm run build

test:
	cd apps/neuro-integration && $(GO) vet ./... && $(GO) test -count=1 ./...
	cd backend/python && $(PYTHON) -m unittest discover -s tests -t .
	cd .. && $(PYTHON) desktop/tools/ci/repo_checks.py

bundle-prod:
	./scripts/bundle/prod.sh

bundle-dev:
	./scripts/bundle/dev.sh

clean:
	rm -rf apps/neuro-integration/dist frontend/dist dist

.PHONY: all build server frontend test bundle-prod bundle-dev clean
