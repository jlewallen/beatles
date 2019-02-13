package main

import (
	"bytes"
	"flag"
	"io"
	"log"
	"os"

	"github.com/zmb3/spotify"
)

type Options struct {
	Dry  bool
	User string
	Name string
	Size int
}

func main() {
	var options Options

	flag.BoolVar(&options.Dry, "dry", false, "dry")
	flag.StringVar(&options.User, "user", "jlewalle", "user")

	flag.Parse()

	log.Printf("Getting playlists for %v", options.User)

	logFile, err := os.OpenFile("beatles.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	defer logFile.Close()
	buffer := new(bytes.Buffer)
	multi := io.MultiWriter(logFile, buffer, os.Stdout)
	log.SetOutput(multi)

	spotifyClient, _ := AuthenticateSpotify()

	// https://open.spotify.com/artist/3WrFJ7ztbogyGnTHbHJFl2?si=BPm1QDocRxW3JkNDNbmGxg

	artistId := spotify.ID("3WrFJ7ztbogyGnTHbHJFl2?si=BPm1QDocRxW3JkNDNbmGxg")
	artist, err := spotifyClient.GetArtist(artistId)
	if err != nil {
		log.Fatalf("Error getting source: %v", err)
	}

	log.Printf("Artist: %v", artist.Name)

	albums, err := GetArtistAlbums(spotifyClient, artist.ID)
	if err != nil {
		log.Fatalf("Error getting source: %v", err)
	}

	for _, album := range albums {
		log.Printf("Album: %v (%v)", album.Name, album.ReleaseDate)

		tracks, err := GetAlbumTracks(spotifyClient, album.ID)
		if err != nil {
			log.Fatalf("Error getting source: %v", err)
		}

		log.Printf("   Tracks: %v", len(tracks))
	}

	/*
		cacher := SpotifyCacher{
			spotifyClient: spotifyClient,
		}

		source, err := GetPlaylist(spotifyClient, options.User, "the beatles (all)")
		if err != nil {
			log.Fatalf("Error getting source: %v", err)
		}

		excluding, err := GetPlaylist(spotifyClient, options.User, "the beatles (excluded)")
		if err != nil {
			log.Fatalf("Error getting excluded: %v", err)
		}

		filtered, err := GetPlaylist(spotifyClient, options.User, "the beatles (filtered)")
		if err != nil {
			log.Fatalf("Error getting filtered: %v", err)
		}

		cacher.Invalidate(source.ID)
		cacher.Invalidate(excluding.ID)
		cacher.Invalidate(filtered.ID)

		sourceTracks, err := cacher.GetPlaylistTracks(options.User, source.ID)
		if err != nil {
			log.Fatalf("Error getting source %v", err)
		}

		excludingTracks, err := cacher.GetPlaylistTracks(options.User, excluding.ID)
		if err != nil {
			log.Fatalf("Error getting excluding %v", err)
		}

		filteredTracks, err := cacher.GetPlaylistTracks(options.User, filtered.ID)
		if err != nil {
			log.Fatalf("Error getting filtered %v", err)
		}

		log.Printf("Have %v tracks excluding %v tracks into filtered (%v tracks)", len(sourceTracks), len(excludingTracks), len(filteredTracks))
	*/
}
