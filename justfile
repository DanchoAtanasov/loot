dev:
  go run main.go

run:
  ./loot

build:
  go build -o loot .

save:
  git add .
  git commit -m "$(date)"
