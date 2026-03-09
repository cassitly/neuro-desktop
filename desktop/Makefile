# ============================================================
# desktop/Makefile
# ============================================================

all: bundle-prod

native:
	cd apps/process-handler && cmake -S . -B build && cmake --build build

go:
	cd apps/neuro-integration && go build -o dist/neuro-integration.exe .

frontend:
	cd frontend && npm install && npm run build

rust:
	cd apps/neuro-desktop && cargo build --release

bundle-dev: rust
	powershell.exe .\scripts\bundle\dev.ps1

bundle-prod: rust
	powershell.exe .\scripts\bundle\prod.ps1

clean:
	rm -rf apps/process-handler/build apps/neuro-integration/dist frontend/dist apps/neuro-desktop/target dist

run: bundle-dev

.PHONY: all native go frontend rust bundle-dev bundle-prod clean run
