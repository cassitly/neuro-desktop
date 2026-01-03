# ============================================================
# desktop/Makefile
# ============================================================

all: bundle-prod

native:
	cd native/c_cpp && cmake -S . -B build && cmake --build build

go:
	cd native/neuro-integration && go build -o neuro-integration main.go

frontend:
	cd frontend && npm install && npm run build

rust:
	cd apps/neuro-desktop && cargo build --release

bundle-dev: rust
	powershell.exe .\scripts\bundle\dev.ps1

bundle-prod: rust
	powershell.exe .\scripts\bundle\prod.ps1

clean:
	rm -rf native/**/build frontend/dist apps/neuro-desktop/target dist

run: bundle-dev

.PHONY: all native go frontend rust bundle-dev bundle-prod clean run
