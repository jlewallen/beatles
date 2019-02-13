package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/zmb3/spotify"
)

type Playlist struct {
	ID   spotify.ID
	User string
	Name string
}

type Track struct {
	ID    spotify.ID
	Title string
}

type PlaylistSet struct {
	Playlists []Playlist
}

func (ps *PlaylistSet) GetAllTracks() (nps *PlaylistSet) {
	return &PlaylistSet{}
}


func GetTrackIds(tracks []spotify.FullTrack) (ids []spotify.ID) {
	for _, track := range tracks {
		ids = append(ids, track.ID)
	}

	return
}

func ToSpotifyIds(ids []interface{}) (ifaces []spotify.ID) {
	for _, id := range ids {
		ifaces = append(ifaces, id.(spotify.ID))
	}
	return
}

func MapIds(ids []spotify.ID) (ifaces []interface{}) {
	for _, id := range ids {
		ifaces = append(ifaces, id)
	}
	return
}

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

	source, err := GetPlaylist(spotifyClient, options.User, "the beatles (all minus revolver and white)")
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
}

type TracksSet struct {
	Ids map[spotify.ID]bool
}

func NewEmptyTracksSet() (ts *TracksSet) {
	ids := make(map[spotify.ID]bool)

	return &TracksSet{
		Ids: ids,
	}
}

func NewTracksSetFromPlaylist(tracks []spotify.PlaylistTrack) (ts *TracksSet) {
	ids := make(map[spotify.ID]bool)

	for _, t := range tracks {
		ids[t.Track.ID] = true
	}

	return &TracksSet{
		Ids: ids,
	}
}

func (ts *TracksSet) MergeInPlace(tracks []spotify.PlaylistTrack) (ns *TracksSet) {
	for _, t := range tracks {
		ts.Ids[t.Track.ID] = true
	}

	return ts
}

func (ts *TracksSet) Remove(removing *TracksSet) (ns *TracksSet) {
	ids := make(map[spotify.ID]bool)

	for k, _ := range ts.Ids {
		if _, ok := removing.Ids[k]; !ok {
			ids[k] = true
		}
	}

	return &TracksSet{
		Ids: ids,
	}
}

func (ts *TracksSet) ToArray() []spotify.ID {
	array := make([]spotify.ID, 0)
	for id, _ := range ts.Ids {
		array = append(array, id)
	}
	return array
}

func removeTracksSetFromPlaylist(spotifyClient *spotify.Client, user string, id spotify.ID, ts *TracksSet) (err error) {
	removals := ts.ToArray()

	for i := 0; i < len(removals); i += 50 {
		batch := removals[i:min(i+50, len(removals))]
		_, err := spotifyClient.RemoveTracksFromPlaylist(id, batch...)
		if err != nil {
			return fmt.Errorf("Error removing tracks: %v", err)
		}
	}

	return nil
}

func addTracksSetToPlaylist(spotifyClient *spotify.Client, user string, id spotify.ID, ts *TracksSet) (err error) {
	additions := ts.ToArray()

	for i := 0; i < len(additions); i += 50 {
		batch := additions[i:min(i+50, len(additions))]
		_, err := spotifyClient.AddTracksToPlaylist(id, batch...)
		if err != nil {
			return fmt.Errorf("Error adding tracks: %v", err)
		}
	}

	return nil
}

func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}
