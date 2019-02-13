package main

import (
	"bytes"
	"flag"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/zmb3/spotify"
)

type Options struct {
	Dry  bool
	User string
	Rebuild bool
}

type TrackInfo struct {
	ID        spotify.ID
	Name      string
	Duration  int
	Dissected []string
}

type ByName []TrackInfo

func (s ByName) Len() int {
	return len(s)
}

func (s ByName) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s ByName) Less(i, j int) bool {
	if strings.Compare(s[i].Name, s[j].Name) > 0 {
		return false
	}
	return true
}

func NewTrackInfo(track spotify.SimpleTrack) TrackInfo {
	return TrackInfo{
		ID:        track.ID,
		Name:      track.Name,
		Duration:  track.Duration,
		Dissected: DisectTrackName(track.Name),
	}
}

func DisectTrackName(name string) []string {
	parts := strings.Split(name, "-")

	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}

	return parts
}

func main() {
	var options Options

	flag.BoolVar(&options.Dry, "dry", false, "dry")
	flag.BoolVar(&options.Rebuild, "rebuild", false, "rebuild")
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

	allTracks := make([]TrackInfo, 0)

	for _, album := range albums {
		tracks, err := cacher.GetAlbumTracks(album.ID)
		if err != nil {
			log.Fatalf("Error getting source: %v", err)
		}

		log.Printf("Album: %v (%v) (%v tracks)", album.Name, album.ReleaseDate, len(tracks))

		for _, track := range tracks {
			allTracks = append(allTracks, NewTrackInfo(track))
		}
	}

	log.Printf("Total Tracks: %v", len(allTracks))

	allTracksPlaylist, err := GetPlaylist(spotifyClient, options.User, "the beatles (all)")
	if err != nil {
		log.Fatalf("Error getting all tracks playlist:: %v", err)
	}

	shortTracksPlaylist, err := GetPlaylist(spotifyClient, options.User, "the beatles (short)")
	if err != nil {
		log.Fatalf("Error getting short tracks playlist: %v", err)
	}

	candidatesTracksPlaylist, err := GetPlaylist(spotifyClient, options.User, "the beatles (candidates)")
	if err != nil {
		log.Fatalf("Error getting candidates tracks playlist: %v", err)
	}

	err = RemoveAllPlaylistTracks(spotifyClient, candidatesTracksPlaylist.ID)
	if err != nil {
		log.Fatalf("Error getting removing tracks: %v", err)
	}

	excludedTracksPlaylist, err := GetPlaylist(spotifyClient, options.User, "the beatles (excluded)")
	if err != nil {
		log.Fatalf("Error getting excluded tracks playlist: %v", err)
	}

	excludedTracks, err := cacher.GetPlaylistTracks(options.User, excludedTracksPlaylist.ID)
	if err != nil {
		log.Fatalf("Error getting excluded tracks: %v", err)
	}

	log.Printf("Have %d excluded tracks", len(excludedTracks))

	excludedTracksSet := NewTracksSetFromPlaylist(excludedTracks)

	sort.Sort(ByName(allTracks))

	titles := make(map[string]bool)
	addingToAll := make([]spotify.ID, 0)
	addingToShort := make([]spotify.ID, 0)
	addingToCandidates := make([]spotify.ID, 0)
	for _, track := range allTracks {
		if _, ok := titles[track.Name]; !ok {
			if track.Duration < 60*1000 {
				addingToShort = append(addingToShort, track.ID)
				if false {
					log.Printf("%v (SHORT)", track)
				}
			} else {
				addingToAll = append(addingToAll, track.ID)
				if false {
					log.Printf("%v", track)
				}
			}

			if !excludedTracksSet.Contains(track.ID) {
				addingToCandidates = append(addingToCandidates, track.ID)
			}

			titles[track.Name] = true
		}
	}

	if options.Rebuild {
		log.Printf("Building '%s'...", allTracksPlaylist.Name)

		err = RemoveAllPlaylistTracks(spotifyClient, allTracksPlaylist.ID)
		if err != nil {
			log.Fatalf("Error getting removing tracks: %v", err)
		}

		err = AddTracksToPlaylist(spotifyClient, allTracksPlaylist.ID, addingToAll)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}

		log.Printf("Building '%s'...", shortTracksPlaylist.Name)

		err = RemoveAllPlaylistTracks(spotifyClient, shortTracksPlaylist.ID)
		if err != nil {
			log.Fatalf("Error getting removing tracks: %v", err)
		}

		err = AddTracksToPlaylist(spotifyClient, shortTracksPlaylist.ID, addingToShort)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}
	}

	log.Printf("Building '%s'...", candidatesTracksPlaylist.Name)

	err = RemoveAllPlaylistTracks(spotifyClient, candidatesTracksPlaylist.ID)
	if err != nil {
		log.Fatalf("Error getting removing tracks: %v", err)
	}

	err = AddTracksToPlaylist(spotifyClient, candidatesTracksPlaylist.ID, addingToCandidates)
	if err != nil {
		log.Fatalf("Error adding tracks: %v", err)
	}

	log.Printf("DONE")
}
