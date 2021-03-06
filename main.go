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
	"time"

	"text/template"

	"github.com/zmb3/spotify"
)

type TrackInfo struct {
	ID                   spotify.ID
	Album                string
	Name                 string
	ShortName            string
	AlbumReleaseDate     time.Time
	SongReleaseDate      time.Time
	Duration             int
	Popularity           int
	Dissected            []string
	Short                bool
	Recordings           int
	Has3OrMoreRecordings bool
	Has1Recording        bool
	Guessed              bool
	Excluded             bool
	ExcludedReasons      []string
	OnExcludedAlbum      bool
	Original             bool
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

type ByReleaseDate []*TrackInfo

func (s ByReleaseDate) Len() int {
	return len(s)
}

func (s ByReleaseDate) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s ByReleaseDate) Less(i, j int) bool {
	return s[i].SongReleaseDate.Unix() > s[j].SongReleaseDate.Unix()
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

func NewTrackInfo(albumName string, albumReleaseDate string, track spotify.FullTrack) *TrackInfo {
	trackName := track.Name
	trackName = strings.Replace(trackName, "U.S.S.R", "U.S.S.R.", -1)
	trackName = strings.Replace(trackName, "Sgt.", "Sgt", -1)
	trackName = strings.Replace(trackName, "Mr.", "Mr", -1)
	trackName = strings.Replace(trackName, "Sixty Four", "Sixty-Four", -1)

	dissected := DissectTrackName(trackName)
	shortName := dissected[0]
	releaseDate, err := time.Parse("2006-01-02", albumReleaseDate)
	if err != nil {
		panic(err)
	}

	return &TrackInfo{
		ID:               track.ID,
		Album:            albumName,
		Name:             trackName,
		ShortName:        shortName,
		Duration:         track.Duration,
		Popularity:       track.Popularity,
		Dissected:        dissected,
		AlbumReleaseDate: releaseDate,
		ExcludedReasons:  make([]string, 0),
	}
}

func (ti *TrackInfo) Exclude(reason string) {
	ti.Excluded = true
	for _, v := range ti.ExcludedReasons {
		if v == reason {
			return
		}
	}
	ti.ExcludedReasons = append(ti.ExcludedReasons, reason)
}

func (ti *TrackInfo) ExcludedReason() string {
	return strings.Join(ti.ExcludedReasons, ", ")
}

func DissectTrackName(name string) []string {
	parts := strings.Split(name, " - ")

	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}

	return parts
}

type Options struct {
	Dry             bool
	User            string
	RebuildMultiple bool
	RebuildBase     bool
	ReadOnlySpotify bool
}

func main() {
	var options Options

	flag.BoolVar(&options.Dry, "dry", false, "dry")
	flag.BoolVar(&options.RebuildBase, "rebuild-base", false, "rebuild")
	flag.BoolVar(&options.RebuildMultiple, "rebuild-multiple", true, "rebuild")
	flag.BoolVar(&options.ReadOnlySpotify, "spotify-ro", false, "spotify-ro")
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

	al := NewAuditLog()

	artistName := "the beatles"
	artistId := spotify.ID("3WrFJ7ztbogyGnTHbHJFl2?si=BPm1QDocRxW3JkNDNbmGxg")
	excludedAlbums := []spotify.ID{
		spotify.ID("3PRoXYsngSwjEQWR5PsHWR"),
		spotify.ID("1klALx0u4AavZNEvC4LrTL"),
		spotify.ID("6QaVfG1pHYl1z15ZxkvVDW"),
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
	trackIdsMap := make(map[spotify.ID]bool)

	for _, album := range albums {
		tracks, err := cacher.GetAlbumTracks(album.ID)
		if err != nil {
			log.Fatalf("Error getting source: %v", err)
		}

		log.Printf("Album: %v (%v) (%v tracks) (%v)", album.Name, album.ReleaseDate, len(tracks), album.ID)

		albumTrackIds := make([]spotify.ID, 0)
		for _, track := range tracks {
			if _, ok := trackIdsMap[track.ID]; !ok {
				allTrackIds = append(allTrackIds, track.ID)
				albumTrackIds = append(albumTrackIds, track.ID)
				trackIdsMap[track.ID] = true
			}
		}

		for i := 0; i < len(albumTrackIds); i += 50 {
			batch := albumTrackIds[i:min(i+50, len(albumTrackIds))]

			fullTracks, err := cacher.GetTracks(batch)
			if err != nil {
				log.Fatalf("Error getting full tracks: %v", err)
			}

			for _, track := range fullTracks {
				allTracks = append(allTracks, NewTrackInfo(album.Name, album.ReleaseDate, track))
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

	sort.Sort(ByName(allTracks))

	allTracksByReleaseDate := make([]*TrackInfo, len(allTracks))
	copy(allTracksByReleaseDate, allTracks)
	sort.Sort(ByReleaseDate(allTracksByReleaseDate))

	log.Printf("Got %v full tracks", len(allFullTracks))

	tracksOnExcludedAlbums := make(map[spotify.ID]string)

	for _, albumId := range excludedAlbums {
		album, err := cacher.GetAlbum(albumId)
		if err != nil {
			log.Fatalf("Error getting album %v: %v", albumId, err)
		}

		tracks, err := cacher.GetAlbumTracks(album.ID)
		if err != nil {
			log.Fatalf("Error getting source: %v", err)
		}

		log.Printf("ExcludedAlbum: %v (%v) (%v tracks)", album.Name, album.ReleaseDate, len(tracks))

		for _, track := range tracks {
			tracksOnExcludedAlbums[track.ID] = album.Name
		}
	}

	playlists, err := cacher.GetPlaylists(options.User)
	if err != nil {
		log.Fatalf("Error getting playlists: %v", err)
	}

	excludedTracks := make(map[spotify.ID]string)
	guessedTracks := make(map[spotify.ID]string)

	for _, playlist := range playlists.Playlists {
		if strings.HasPrefix(playlist.Name, artistName) {
			if strings.Contains(playlist.Name, "(excluded") {
				cacher.Invalidate(playlist.ID)

				playlistTracks, err := cacher.GetPlaylistTracks(options.User, playlist.ID)
				if err != nil {
					log.Fatalf("Error getting tracks: %v", err)
				}

				log.Printf("Applying exclusion playlist '%s' (%v tracks)", playlist.Name, len(playlistTracks))

				for _, track := range playlistTracks {
					excludedTracks[track.Track.ID] = playlist.Name
					if strings.Contains(playlist.Name, "guessed") {
						guessedTracks[track.Track.ID] = playlist.Name
					}
				}
			}
		}
	}

	log.Printf("Have %d excluded tracks", len(excludedTracks))
	log.Printf("Have %d tracks from excluded albums", len(tracksOnExcludedAlbums))

	byShortNames := make(map[string][]*TrackInfo)
	addingToAll := make([]spotify.ID, 0)
	addingToShort := make([]spotify.ID, 0)
	addingToExcluded := make([]spotify.ID, 0)
	for _, track := range allTracks {
		if reason, ok := excludedTracks[track.ID]; ok {
			addingToExcluded = append(addingToExcluded, track.ID)
			reason := fmt.Sprintf("Excluded by %s", reason)
			track.Exclude(reason)
			al.Append(track.Name, reason)
		}

		if _, ok := guessedTracks[track.ID]; ok {
			track.Guessed = true
		}

		if track.Duration < 60*1000 {
			addingToShort = append(addingToShort, track.ID)
			reason := fmt.Sprintf("Too short (%vs)", track.Duration/1000.0)
			track.Exclude(reason)
			al.Append(track.Name, reason)
		}

		if _, ok := byShortNames[track.ShortName]; !ok {
			byShortNames[track.ShortName] = make([]*TrackInfo, 0)
		}

		byShortNames[track.ShortName] = append(byShortNames[track.ShortName], track)

		addingToAll = append(addingToAll, track.ID)
	}

	for _, v := range byShortNames {
		songReleaseDate := v[0].AlbumReleaseDate

		for _, track := range v {
			track.Recordings = len(v)

			if track.AlbumReleaseDate.Before(songReleaseDate) {
				songReleaseDate = track.AlbumReleaseDate
			}

			if albumName, ok := tracksOnExcludedAlbums[track.ID]; ok {
				for _, track := range v {
					track.OnExcludedAlbum = true
					reason := fmt.Sprintf("Excluded album (%v)", albumName)
					track.Exclude(reason)
					al.Append(track.Name, reason)
				}
			}
		}

		for _, track := range v {
			track.SongReleaseDate = songReleaseDate
		}

		for _, track := range v {
			if track.AlbumReleaseDate == songReleaseDate {
				al.Append(track.Name, fmt.Sprintf("Marked as original (%v)", track.Album))
				track.Original = true
				break
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
		} else {
			for _, track := range v {
				reason := fmt.Sprintf("Too few recordings (%v)", len(v))
				track.Exclude(reason)
				al.Append(track.Name, reason)
			}
		}
	}

	err = GenerateTable(allTracks)
	if err != nil {
		log.Fatalf("Error generating table: %v", err)
	}

	if options.RebuildMultiple {
		addingToExcluded := make([]spotify.ID, 0)
		addingTo3OrMore := make([]spotify.ID, 0)
		addingTo3OrMoreUnfiltered := make([]spotify.ID, 0)

		for _, track := range allTracks {
			if track.Has3OrMoreRecordings {
				if track.OnExcludedAlbum {
					addingToExcluded = append(addingToExcluded, track.ID)
				} else {
					if !track.Excluded {
						addingTo3OrMore = append(addingTo3OrMore, track.ID)
					}

					if !track.Excluded || track.Excluded && !track.Guessed {
						addingTo3OrMoreUnfiltered = append(addingTo3OrMoreUnfiltered, track.ID)
					}
				}
			}
		}

		byReleaseDate := make([]spotify.ID, 0)
		originals := make([]spotify.ID, 0)
		originalsUnfiltered := make([]spotify.ID, 0)

		for _, track := range allTracksByReleaseDate {
			if track.Has3OrMoreRecordings {
				if !track.Excluded {
					byReleaseDate = append(byReleaseDate, track.ID)

					if track.Original {
						originals = append(originals, track.ID)
					}
				}
				if track.Original {
					if !track.Excluded || track.Excluded && !track.Guessed {
						originalsUnfiltered = append(originalsUnfiltered, track.ID)
					}
				}
			}
		}

		err = MaybeSetPlaylistTracksByName(spotifyClient, options.ReadOnlySpotify, options.User, artistName+" (R >= 3 unfiltered)", addingTo3OrMoreUnfiltered)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}

		err = MaybeSetPlaylistTracksByName(spotifyClient, options.ReadOnlySpotify, options.User, artistName+" (R >= 3)", addingTo3OrMore)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}

		err = MaybeSetPlaylistTracksByName(spotifyClient, options.ReadOnlySpotify, options.User, artistName+" (R >= 3 originals)", originals)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}

		err = MaybeSetPlaylistTracksByName(spotifyClient, options.ReadOnlySpotify, options.User, artistName+" (R >= 3 originals unfiltered)", originalsUnfiltered)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}

		err = MaybeSetPlaylistTracksByName(spotifyClient, options.ReadOnlySpotify, options.User, artistName+" (R >= 3 by release date)", byReleaseDate)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}

		err = MaybeSetPlaylistTracksByName(spotifyClient, options.ReadOnlySpotify, options.User, artistName+" (R >= 3 on excluded albums)", addingToExcluded)
		if err != nil {
			log.Fatalf("Error adding tracks: %v", err)
		}

		if options.RebuildBase {
			err = MaybeSetPlaylistTracksByName(spotifyClient, options.ReadOnlySpotify, options.User, artistName+" (all)", addingToAll)
			if err != nil {
				log.Fatalf("Error adding tracks: %v", err)
			}

			err = MaybeSetPlaylistTracksByName(spotifyClient, options.ReadOnlySpotify, options.User, artistName+" (short)", addingToShort)
			if err != nil {
				log.Fatalf("Error adding tracks: %v", err)
			}
		}
	}

	al.Write("data/audit.org")

	log.Printf("DONE")
}

func GenerateTable(tracks []*TrackInfo) error {
	templates := map[string]string{
		"tracks.org.template":     "tracks.org",
		"excluded.org.template":   "excluded.org",
		"candidates.org.template": "candidates.org",
		"all.org.template":        "all.org",
	}
	byPopularity := make([]*TrackInfo, len(tracks))

	copy(byPopularity, tracks)

	sort.Sort(ByPopularity(byPopularity))

	for templateName, fileName := range templates {
		templateData, err := ioutil.ReadFile(filepath.Join("./", templateName))
		if err != nil {
			return err
		}

		template, err := template.New(fileName).Parse(string(templateData))
		if err != nil {
			return err
		}

		path := filepath.Join("./data", fileName)
		log.Printf("Writing %s", path)

		file, err := os.Create(path)
		if err != nil {
			return err
		}

		defer file.Close()

		data := struct {
			ByName       []*TrackInfo
			ByPopularity []*TrackInfo
		}{
			tracks,
			byPopularity,
		}

		err = template.Execute(file, data)
		if err != nil {
			return err
		}
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
