beatles: *.go
	go build -o $@ $^

clean:
	rm -f beatles

fmt:
	go fmt *.go
