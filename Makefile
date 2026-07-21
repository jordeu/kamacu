.PHONY: build frontend backend dev-backend dev-frontend test clean

build: frontend backend

frontend:
	cd web && npm install && npm run build

backend:
	go build -o bin/kamacu ./cmd/kamacu

test:
	go test ./...

dev-backend:
	go run ./cmd/kamacu serve

dev-frontend:
	cd web && npm run dev

clean:
	rm -rf bin web/dist/assets
