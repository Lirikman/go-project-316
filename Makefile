build: # сборка утилиты
	go build -o bin/crawler ./cmd/hexlet-go-crawler

lint: # проверка кода линтером golangci-lint
	golangci-lint run
	
test: # запуск тестов
	go test -v ./...

run: # запуск запроса к введённому url
	go run ./cmd/hexlet-go-crawler/main.go $(URL) || true