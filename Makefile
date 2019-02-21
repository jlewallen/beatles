beatles: *.go
	go build -o $@ $^

clean:
	rm -f beatles

docs:
	./beatles --spotify-ro
	pandoc all.org > all.html
	pandoc excluded.org > excluded.html
	pandoc candidates.org > candidates.html
	pandoc tracks.org > tracks.html
	pandoc audit.org > audit.html

fmt:
	go fmt *.go
