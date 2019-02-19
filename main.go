package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"text/template"

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
	ID                   spotify.ID
	Album                string
	Name                 string
	ShortName            string
	Duration             int
	Popularity           int
	Dissected            []string
	Short                bool
	Recordings           int
	Has3OrMoreRecordings bool
	Has1Recording        bool
	Excluded             bool
	OnExcludedAlbum      bool
}

type ByName []*TrackInfo

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

type ByPopularity []*TrackInfo

func (s ByPopularity) Len() int {
	return len(s)
}

func (s ByPopularity) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s ByPopularity) Less(i, j int) bool {
	return s[i].Popularity > s[j].Popularity
}

func NewTrackInfo(albumName string, track spotify.FullTrack) *TrackInfo {
	dissected := DisectTrackName(track.Name)
	shortName := dissected[0]

	return &TrackInfo{
		ID:         track.ID,
		Album:      albumName,
		Name:       track.Name,
		ShortName:  shortName,
		Duration:   track.Duration,
		Popularity: track.Popularity,
		Dissected:  dissected,
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

	al := NewAuditLog()

	artistName := "the beatles"
	artistId := spotify.ID("3WrFJ7ztbogyGnTHbHJFl2?si=BPm1QDocRxW3JkNDNbmGxg")
	excludedAlbums := []spotify.ID{
		spotify.ID("3PRoXYsngSwjEQWR5PsHWR?si=aLXprrjrQG-aKNfk9TUTGg"),
		spotify.ID("1WMVvswNzB9i2UMh9svso5?si=4Aie4TyLQ5eHyTCuNcdymg"),
	}

	artist, err := spotifyClient.GetArtist(artistId)
	if err != nil {
		log.Fatalf("Error getting source: %v", err)
	}

	log.Printf("Artist: %v", artist.Name)

	albums, err := cacher.GetArtistAlbums(artist.ID)
	if err != nil {
		log.Fatalf("Error getting source: %v", err)
	}

	allTracks := make([]*TrackInfo, 0)
	allTrackIds := make([]spotify.ID, 0)

	for _, album := range albums {
		tracks, err := cacher.GetAlbumTracks(album.ID)
		if err != nil {
			log.Fatalf("Error getting source: %v", err)
		}

		log.Printf("Album: %v (%v) (%v tracks)", album.Name, album.ReleaseDate, len(tracks))

		albumTrackIds := make([]spotify.ID, 0)
		for _, track := range tracks {
			allTrackIds = append(allTrackIds, track.ID)
			albumTrackIds = append(albumTrackIds, track.ID)
		}

		for i := 0; i < len(albumTrackIds); i += 50 {
			batch := albumTrackIds[i:min(i+50, len(albumTrackIds))]

			fullTracks, err := cacher.GetTracks(batch)
			if err != nil {
				log.Fatalf("Error getting full tracks: %v", err)
			}

			for _, track := range fullTracks {
				allTracks = append(allTracks, NewTrackInfo(album.Name, track))
			}
		}
	}

	allFullTracks := make([]spotify.FullTrack, 0)

	for i := 0; i < len(allTrackIds); i += 50 {
		batch := allTrackIds[i:min(i+50, len(allTrackIds))]
		fullTracks, err := cacher.GetTracks(batch)
		if err != nil {
			log.Fatalf("Error getting full tracks: %v", err)
		}

		allFullTracks = append(allFullTracks, fullTracks...)
	}

	log.Printf("Got %v full tracks", len(allFullTracks))

	tracksOnExcludedAlbums := NewEmptyTracksSet()

	for _, albumId := range excludedAlbums {
		album, err := cacher.GetAlbum(albumId)
		if err != nil {
			log.Fatalf("Error getting source: %v", err)
		}

		tracks, err := cacher.GetAlbumTracks(album.ID)
		if err != nil {
			log.Fatalf("Error getting source: %v", err)
		}

		log.Printf("Album(EXCLUDED): %v (%v) (%v tracks)", album.Name, album.ReleaseDate, len(tracks))

		for _, track := range tracks {
			tracksOnExcludedAlbums.Add(track.ID)
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

	byShortNames := make(map[string][]*TrackInfo)
	byTitles := make(map[string]bool)
	addingToAll := make([]spotify.ID, 0)
	addingToShort := make([]spotify.ID, 0)
	addingToCandidates := make([]spotify.ID, 0)
	for _, track := range allTracks {
		if excludedTracksSet.Contains(track.ID) {
			track.Excluded = true
			al.Append(track.Name, "Excluded")

		}

		if _, ok := byTitles[track.Name]; !ok {
			if track.Duration < 60*1000 {
				addingToShort = append(addingToShort, track.ID)
				al.Append(track.Name, "Short")
			} else {
				addingToAll = append(addingToAll, track.ID)

				if !track.Excluded {
					addingToCandidates = append(addingToCandidates, track.ID)

					if _, ok := byShortNames[track.ShortName]; !ok {
						byShortNames[track.ShortName] = make([]*TrackInfo, 0)
					}

					byShortNames[track.ShortName] = append(byShortNames[track.ShortName], track)
				}
			}

			byTitles[track.Name] = true
		}
	}

	for _, v := range byShortNames {
		for _, track := range v {
			track.Recordings = len(v)

			if tracksOnExcludedAlbums.Contains(track.ID) {
				for _, track := range v {
					track.OnExcludedAlbum = true
					al.Append(track.Name, fmt.Sprintf("Excluded album (%v)", track.Album))
				}
			}
		}

		if len(v) == 1 {
			for _, track := range v {
				track.Has1Recording = true
			}
		}
		if len(v) >= 3 {
			for _, track := range v {
				track.Has3OrMoreRecordings = true
			}
		}
	}

	err = GenerateTable(allTracks)
	if err != nil {
		log.Fatalf("Error generating table: %v", err)
	}

	if options.RebuildMultiple {
		multipleRecordingsPlaylist, err := GetPlaylist(spotifyClient, options.User, artistName+" (3 or more recordings)")
		if err != nil {
			log.Fatalf("Error getting multiple recordings playlist: %v", err)
		}

		addingTo3OrMore := make([]spotify.ID, 0)
		for _, track := range allTracks {
			if !track.Excluded {
				if track.Has3OrMoreRecordings {
					addingTo3OrMore = append(addingTo3OrMore, track.ID)
				}
			}
		}

		err = SetPlaylistTracks(spotifyClient, multipleRecordingsPlaylist.ID, addingTo3OrMore)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}
	}

	if options.RebuildMultiple {
		multipleRecordingsPlaylist, err := GetPlaylist(spotifyClient, options.User, artistName+" (3 or more recordings w/o excluded albums)")
		if err != nil {
			log.Fatalf("Error getting multiple recordings playlist: %v", err)
		}

		addingTo3OrMore := make([]spotify.ID, 0)
		for _, track := range allTracks {
			if !track.Excluded && !track.OnExcludedAlbum {
				if track.Has3OrMoreRecordings {
					addingTo3OrMore = append(addingTo3OrMore, track.ID)
				}
			}
		}

		err = SetPlaylistTracks(spotifyClient, multipleRecordingsPlaylist.ID, addingTo3OrMore)
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
		for _, track := range allTracks {
			if track.Has1Recording {
				addingToSingles = append(addingToSingles, track.ID)
			}
		}

		err = SetPlaylistTracks(spotifyClient, singlesTracksPlaylist.ID, addingToSingles)
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

		err = SetPlaylistTracks(spotifyClient, shortTracksPlaylist.ID, addingToShort)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}
	}

	log.Printf("Building '%s'...", candidatesTracksPlaylist.Name)

	err = SetPlaylistTracks(spotifyClient, candidatesTracksPlaylist.ID, addingToCandidates)
	if err != nil {
		log.Fatalf("Error adding tracks: %v", err)
	}

	al.Write("audit.org")

	log.Printf("DONE")
}

func GenerateTable(tracks []*TrackInfo) error {
	byPopularity := make([]*TrackInfo, len(tracks))

	copy(byPopularity, tracks)

	sort.Sort(ByPopularity(byPopularity))

	templateData, err := ioutil.ReadFile(filepath.Join("./", "tracks.org.template"))
	if err != nil {
		return err
	}

	template, err := template.New("tracks.org").Parse(string(templateData))
	if err != nil {
		return err
	}

	path := filepath.Join("./", "tracks.org")
	log.Printf("Writing %s", path)

	file, err := os.Create(path)
	if err != nil {
		return err
	}

	defer file.Close()

	data := struct {
		ByName []*TrackInfo
		ByPopularity []*TrackInfo
	}{
		tracks,
		byPopularity,
	}

	err = template.Execute(file, data)
	if err != nil {
		return err
	}

	return nil
}

type AuditEntry struct {
	Track  string
	Reason string
}

type AuditLog struct {
	Entries []AuditEntry
}

func NewAuditLog() *AuditLog {
	return &AuditLog{
		Entries: make([]AuditEntry, 0),
	}
}

func (al *AuditLog) Append(track, reason string) {
	al.Entries = append(al.Entries, AuditEntry{
		Track:  track,
		Reason: reason,
	})
}

func (al *AuditLog) Write(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	defer f.Close()

	for _, entry := range al.Entries {
		f.WriteString(fmt.Sprintf("| %s | %s |\n", entry.Track, entry.Reason))
	}

	return nil
}
