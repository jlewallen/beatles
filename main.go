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
	Dry             bool
	User            string
	RebuildSingles  bool
	RebuildMultiple bool
	RebuildBase     bool
}

type TrackInfo struct {
	ID        spotify.ID
	Name      string
	ShortName string
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
	dissected := DisectTrackName(track.Name)
	shortName := dissected[0]

	return TrackInfo{
		ID:        track.ID,
		Name:      track.Name,
		ShortName: shortName,
		Duration:  track.Duration,
		Dissected: dissected,
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
	flag.BoolVar(&options.RebuildSingles, "rebuild-singles", false, "rebuild")
	flag.BoolVar(&options.RebuildBase, "rebuild-base", false, "rebuild")
	flag.BoolVar(&options.RebuildMultiple, "rebuild-multiple", true, "rebuild")
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

	artistName := "the beatles"
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

	playlists, err := cacher.GetPlaylists(options.User)
	if err != nil {
		log.Fatalf("Error getting playlists: %v", err)
	}

	allTracksPlaylist, err := GetPlaylist(spotifyClient, options.User, artistName+" (all)")
	if err != nil {
		log.Fatalf("Error getting all tracks playlist: %v", err)
	}

	shortTracksPlaylist, err := GetPlaylist(spotifyClient, options.User, artistName+" (short)")
	if err != nil {
		log.Fatalf("Error getting short tracks playlist: %v", err)
	}

	candidatesTracksPlaylist, err := GetPlaylist(spotifyClient, options.User, artistName+" (candidates)")
	if err != nil {
		log.Fatalf("Error getting candidates tracks playlist: %v", err)
	}

	err = RemoveAllPlaylistTracks(spotifyClient, candidatesTracksPlaylist.ID)
	if err != nil {
		log.Fatalf("Error getting removing tracks: %v", err)
	}

	excludedTracksSet := NewEmptyTracksSet()

	for _, playlist := range playlists.Playlists {
		if strings.HasPrefix(playlist.Name, artistName) {
			if strings.Contains(playlist.Name, "excluded") {
				log.Printf("Applying exclusion playlist '%s'...", playlist.Name)

				cacher.Invalidate(playlist.ID)

				excludedTracks, err := cacher.GetPlaylistTracks(options.User, playlist.ID)
				if err != nil {
					log.Fatalf("Error getting tracks: %v", err)
				}

				excludedTracksSet.MergeInPlace(excludedTracks)
			}
		}
	}

	log.Printf("Have %d excluded tracks", len(excludedTracksSet.ToArray()))

	sort.Sort(ByName(allTracks))

	byShortNames := make(map[string][]TrackInfo)
	byTitles := make(map[string]bool)
	addingToAll := make([]spotify.ID, 0)
	addingToShort := make([]spotify.ID, 0)
	addingToCandidates := make([]spotify.ID, 0)
	for _, track := range allTracks {
		if _, ok := byTitles[track.Name]; !ok {
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

				if !excludedTracksSet.Contains(track.ID) {
					addingToCandidates = append(addingToCandidates, track.ID)

					if _, ok := byShortNames[track.ShortName]; !ok {
						byShortNames[track.ShortName] = make([]TrackInfo, 0)
					}

					byShortNames[track.ShortName] = append(byShortNames[track.ShortName], track)
				}
			}

			byTitles[track.Name] = true
		}
	}

	if options.RebuildMultiple {
		multipleRecordingsPlaylist, err := GetPlaylist(spotifyClient, options.User, artistName+" (3 or more recordings)")
		if err != nil {
			log.Fatalf("Error getting multiple recordings playlist: %v", err)
		}

		has3OrMoreRecordsings := make(map[spotify.ID]bool)
		for _, v := range byShortNames {
			if len(v) >= 3 {
				for _, track := range v {
					has3OrMoreRecordsings[track.ID] = true
				}
			}
		}

		addingTo3OrMore := make([]spotify.ID, 0)
		for _, track := range allTracks {
			if !excludedTracksSet.Contains(track.ID) {
				if _, ok := has3OrMoreRecordsings[track.ID]; ok {
					addingTo3OrMore = append(addingTo3OrMore, track.ID)
				}
			}
		}

		err = RemoveAllPlaylistTracks(spotifyClient, multipleRecordingsPlaylist.ID)
		if err != nil {
			log.Fatalf("Error getting removing tracks: %v", err)
		}

		err = AddTracksToPlaylist(spotifyClient, multipleRecordingsPlaylist.ID, addingTo3OrMore)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}
	}

	if options.RebuildSingles {
		singlesTracksPlaylist, err := GetPlaylist(spotifyClient, options.User, artistName+" (excluded - single recordings)")
		if err != nil {
			log.Fatalf("Error getting singles tracks playlist: %v", err)
		}

		addingToSingles := make([]spotify.ID, 0)

		for k, v := range byShortNames {
			if len(v) == 1 {
				for _, track := range v {
					addingToSingles = append(addingToSingles, track.ID)
				}
				log.Printf("%v %v", k, len(v))
			}
		}

		err = RemoveAllPlaylistTracks(spotifyClient, singlesTracksPlaylist.ID)
		if err != nil {
			log.Fatalf("Error getting removing tracks: %v", err)
		}

		err = AddTracksToPlaylist(spotifyClient, singlesTracksPlaylist.ID, addingToSingles)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}
	}

	if options.RebuildBase {
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
