all: docs

beatles: *.go
	go build -o $@ $^

data/all.org: beatles
	./beatles --spotify-ro

docs: data/all.html data/tracks.html data/excluded.html data/audit.html data/candidates.html

%.html: %.org
	pandoc $^ > $@

fmt:
	go fmt *.go

clean:
	rm -f beatles
	rm -f data/*.org data/*.html
