

default:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o image-migration

clean:
	rm image-migration