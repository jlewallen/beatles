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

	cacher := SpotifyCacher{
		spotifyClient: spotifyClient,
	}

	// https://open.spotify.com/artist/3WrFJ7ztbogyGnTHbHJFl2?si=BPm1QDocRxW3JkNDNbmGxg

	artistId := spotify.ID("3WrFJ7ztbogyGnTHbHJFl2?si=BPm1QDocRxW3JkNDNbmGxg")
	artist, err := spotifyClient.GetArtist(artistId)
	if err != nil {
		log.Fatalf("Error getting source: %v", err)
	}

	log.Printf("Artist: %v", artist.Name)

	albums, err := cacher.GetArtistAlbums(artist.ID)
	if err != nil {
		log.Fatalf("Error getting source: %v", err)
	}

	allTracks := make([]spotify.SimpleTrack, 0)

	for _, album := range albums {
		log.Printf("Album: %v (%v)", album.Name, album.ReleaseDate)

		tracks, err := cacher.GetAlbumTracks(album.ID)
		if err != nil {
			log.Fatalf("Error getting source: %v", err)
		}

		log.Printf("   Tracks: %v", len(tracks))

		for _, track := range tracks {
			log.Printf("   Track: %v", track.Name)

			allTracks = append(allTracks, track)
		}
	}

	log.Printf("Total Tracks: %v", len(allTracks))

	allTracksPlaylist, err := GetPlaylist(spotifyClient, options.User, "the beatles (all)")
	if err != nil {
		log.Fatalf("Error getting source: %v", err)
	}

	cacher.Invalidate(allTracksPlaylist.ID)

	allTracksPlaylistTracks, err := cacher.GetPlaylistTracks(options.User, allTracksPlaylist.ID)
	if err != nil {
		log.Fatalf("Error getting source %v", err)
	}

	update := NewPlaylistUpdate(GetTrackIdsFromPlaylistTracks(allTracksPlaylistTracks))

	for _, track := range allTracks {
		update.AddTrack(track.ID)
	}

	adding := update.GetIdsToAdd()
	removing := update.GetIdsToRemove()

	log.Printf("Modifying all tracks: %d adding %v removing", len(adding.ToArray()), len(removing.ToArray()))

	if false {
		excluding, err := GetPlaylist(spotifyClient, options.User, "the beatles (excluded)")
		if err != nil {
			log.Fatalf("Error getting excluded: %v", err)
		}

		filtered, err := GetPlaylist(spotifyClient, options.User, "the beatles (filtered)")
		if err != nil {
			log.Fatalf("Error getting filtered: %v", err)
		}

		cacher.Invalidate(excluding.ID)
		cacher.Invalidate(filtered.ID)

		excludingTracks, err := cacher.GetPlaylistTracks(options.User, excluding.ID)
		if err != nil {
			log.Fatalf("Error getting excluding %v", err)
		}

		filteredTracks, err := cacher.GetPlaylistTracks(options.User, filtered.ID)
		if err != nil {
			log.Fatalf("Error getting filtered %v", err)
		}

		log.Printf("Have %v tracks excluding %v tracks into filtered (%v tracks)", len(allTracksPlaylistTracks), len(excludingTracks), len(filteredTracks))
	}
}
