.PHONY: build frontend backend dev-backend dev-frontend test clean

build: frontend backend

frontend:
	cd web && npm install && npm run build

backend:
	go build -o bin/kangent ./cmd/kangent

test:
	go test ./...

dev-backend:
	go run ./cmd/kangent

dev-frontend:
	cd web && npm run dev

clean:
	rm -rf bin web/dist/assets
