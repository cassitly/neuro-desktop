all: native frontend rust

native:
	cd native/c_cpp && cmake -S . -B build && cmake --build build
	cd native/go && go build

frontend:
	cd frontend && npm install && npm run build

rust:
	cd apps/neuro-desktop && cargo build --release

clean:
	rm -rf native/**/build frontend/dist

run: native frontend rust
	cd apps/neuro-desktop && cargo run --release
