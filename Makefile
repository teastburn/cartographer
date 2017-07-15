fmt:
	go fmt $(shell glide nv)

dev:
	docker-compose up -d
	docker-compose logs -f --tail=100

info:
	curl --http2 -k https://localhost:8080/info

write:
	curl --http2 -k -XPOST -H 'Content-Type: application/json' -d '{"lat":123.456,"lon":123.456}' https://localhost:8080/geo

cities:
	go run tools/import/main.go -f cities23krand.csv
