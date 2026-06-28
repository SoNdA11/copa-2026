.PHONY: build run clean dev

build:
	go build -o bin/copa ./cmd/web/

run: build
	./bin/copa

dev:
	go run ./cmd/web/

clean:
	rm -f bin/copa data/copa.db

seed:
	@mkdir -p data/seed
	curl -sL -o data/seed/football.teams.json "https://raw.githubusercontent.com/rezarahiminia/worldcup2026/main/football.teams.json"
	curl -sL -o data/seed/football.matches.json "https://raw.githubusercontent.com/rezarahiminia/worldcup2026/main/football.matches.json"
	curl -sL -o data/seed/football.stadiums.json "https://raw.githubusercontent.com/rezarahiminia/worldcup2026/main/football.stadiums.json"
	curl -sL -o data/seed/football.matchtables.json "https://raw.githubusercontent.com/rezarahiminia/worldcup2026/main/football.matchtables.json"
	@echo "Seed data downloaded!"
