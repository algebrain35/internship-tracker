# Build both binaries.
# CGo is required for mattn/go-sqlite3 (the SQLite driver).
# The replace directive in go.mod points to ./vendor/github.com/mattn/go-sqlite3
# so go build -mod=mod resolves it from the local vendor copy.

.PHONY: build crawler server clean

build: crawler server

crawler:
	go build -mod=mod -o bin/crawler ./cmd/crawler

server:
	go build -mod=mod -o bin/server ./cmd/server

clean:
	rm -rf bin/ jobs.db

# Run the crawler once, then start the server.
# Open http://localhost:8080 while the crawler runs to watch jobs arrive.
run:
	mkdir -p bin
	$(MAKE) build
	./bin/crawler &
	./bin/server
