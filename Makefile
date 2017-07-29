NR_KEY=""

fmt:
	go fmt $(shell glide nv)

dev:
	NR_KEY=${NR_KEY} docker-compose up -d
	docker-compose logs -f --tail=100

glide:
	docker-compose exec cartographer glide update
	docker-compose exec cartographer glide install

info:
	curl --http2 -k https://localhost:8080/info

write:
	curl --http2 -k -XPOST -H 'Content-Type: application/json' -d '{"lat":123.456,"lon":123.456}' https://localhost:8080/geo

cities:
	go build -o ./import tools/import/main.go
	time ./import -f cities23krand.csv

cities5:
	go build -o ./import tools/import/main.go
	time ./import -f cities5.csv
