package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"net/http"

	"golang.org/x/oauth2"

	mapset "github.com/deckarep/golang-set"

	"github.com/zmb3/spotify"
)

var (
	authenticator = spotify.NewAuthenticator(spotifyRedirectUrl, spotify.ScopePlaylistModifyPrivate, spotify.ScopePlaylistModifyPublic, spotify.ScopeUserLibraryModify, spotify.ScopeUserReadPrivate)
	clientChannel = make(chan *spotify.Client)
)

func AuthenticateSpotify() (spotifyClient *spotify.Client, err error) {
	var tokens = ReadTokens()

	log.Printf("Authenticating with Spotify...")

	if tokens.Spotify.AccessToken == "" {
		http.HandleFunc("/spotify/callback", CompleteAuth)
		go http.ListenAndServe(":9090", nil)

		authenticator.SetAuthInfo(spotifyClientId, spotifyClientSecret)

		url := authenticator.AuthURL(spotifyOauthStateString)
		log.Println("Please log in to Spotify by visiting the following page in your browser:", url)

		spotifyClient = <-clientChannel
	} else {
		var oauthToken oauth2.Token
		oauthToken.AccessToken = tokens.Spotify.AccessToken
		oauthToken.RefreshToken = tokens.Spotify.RefreshToken
		oauthToken.Expiry, _ = time.Parse("Mon Jan 2 15:04:05 -0700 MST 2006", tokens.Spotify.Expiry)
		oauthToken.TokenType = tokens.Spotify.TokenType
		newClient := authenticator.NewClient(&oauthToken)
		authenticator.SetAuthInfo(spotifyClientId, spotifyClientSecret)
		spotifyClient = &newClient
	}

	user, err := spotifyClient.CurrentUser()
	if err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	log.Println("spotify: You are logged in as", user.ID)

	return
}

func CompleteAuth(w http.ResponseWriter, r *http.Request) {
	token, err := authenticator.Token(spotifyOauthStateString, r)
	if err != nil {
		http.Error(w, "Unable to get token", http.StatusForbidden)
		log.Fatal(err)
	}

	if actualState := r.FormValue("state"); actualState != spotifyOauthStateString {
		http.NotFound(w, r)
		log.Fatalf("State mismatch: %s != %s\n", actualState, spotifyOauthStateString)
	}

	var tokens = ReadTokens()
	tokens.Spotify.AccessToken = token.AccessToken
	tokens.Spotify.RefreshToken = token.RefreshToken
	tokens.Spotify.Expiry = token.Expiry.Format("Mon Jan 2 15:04:05 -0700 MST 2006")
	tokens.Spotify.TokenType = token.TokenType
	WriteTokens(tokens)

	client := authenticator.NewClient(token)
	clientChannel <- &client
}

func GetPlaylistByTitle(spotifyClient *spotify.Client, user, name string) (*spotify.SimplePlaylist, error) {
	limit := 20
	offset := 0
	options := spotify.Options{Limit: &limit, Offset: &offset}
	for {
		playlists, err := spotifyClient.GetPlaylistsForUserOpt(user, &options)
		if err != nil {
			return nil, fmt.Errorf("Unable to get playlists: %v", err)
		}

		for _, iter := range playlists.Playlists {
			if strings.EqualFold(iter.Name, name) {
				return &iter, nil
			}
		}

		if len(playlists.Playlists) < *options.Limit {
			break
		}

		offset := *options.Limit + *options.Offset
		options.Offset = &offset
	}

	return nil, nil
}

func GetPlaylist(spotifyClient *spotify.Client, user string, name string) (pl *spotify.SimplePlaylist, err error) {
	pl, err = GetPlaylistByTitle(spotifyClient, user, name)
	if err != nil {
		return nil, fmt.Errorf("Error getting '%s': %v", name, err)
	}
	if pl == nil {
		created, err := spotifyClient.CreatePlaylistForUser(user, name, "description", true)
		if err != nil {
			return nil, fmt.Errorf("Unable to create playlist: %v", err)
		}

		log.Printf("Created playlist: %v", created)

		pl, err = GetPlaylistByTitle(spotifyClient, user, name)
		if err != nil {
			return nil, fmt.Errorf("Error getting %s: %v", name, err)
		}
	}

	return pl, nil
}

type PlaylistUpdate struct {
	idsBefore mapset.Set
	idsAfter  []spotify.ID
}

func NewPlaylistUpdate(idsBefore []spotify.ID) *PlaylistUpdate {
	return &PlaylistUpdate{
		idsBefore: mapset.NewSetFromSlice(MapIds(idsBefore)),
		idsAfter:  make([]spotify.ID, 0),
	}
}

func (pu *PlaylistUpdate) AddTrack(id spotify.ID) {
	pu.idsAfter = append(pu.idsAfter, id)
}

func (pu *PlaylistUpdate) GetIdsToRemove() *TracksSet {
	afterSet := mapset.NewSetFromSlice(MapIds(pu.idsAfter))
	idsToRemove := pu.idsBefore.Difference(afterSet)
	return NewTracksSet(ToSpotifyIds(idsToRemove.ToSlice()))
}

func (pu *PlaylistUpdate) GetIdsToAdd() *TracksSet {
	ids := make([]spotify.ID, 0)
	for _, id := range pu.idsAfter {
		if !pu.idsBefore.Contains(id) {
			ids = append(ids, id)
		}
	}
	return NewTracksSet(ids)
}

func (pu *PlaylistUpdate) MergeBeforeAndToAdd() {
	for _, id := range pu.idsAfter {
		pu.idsBefore.Add(id)
	}
}

func GetArtistAlbums(spotifyClient *spotify.Client, id spotify.ID) ([]spotify.SimpleAlbum, error) {
	all := make([]spotify.SimpleAlbum, 0)
	limit := 20
	offset := 0
	options := spotify.Options{Limit: &limit, Offset: &offset}
	for {
		albums, err := spotifyClient.GetArtistAlbumsOpt(id, &options, nil)
		if err != nil {
			return nil, fmt.Errorf("Unable to get albums: %v", err)
		}

		all = append(all, albums.Albums...)

		if len(albums.Albums) < *options.Limit {
			break
		}

		offset := *options.Limit + *options.Offset
		options.Offset = &offset
	}

	return all, nil
}

func GetAlbumTracks(spotifyClient *spotify.Client, id spotify.ID) ([]spotify.SimpleTrack, error) {
	all := make([]spotify.SimpleTrack, 0)
	limit := 20
	offset := 0
	for {
		tracks, err := spotifyClient.GetAlbumTracksOpt(id, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("Unable to get tracks: %v", err)
		}

		all = append(all, tracks.Tracks...)

		if len(tracks.Tracks) < limit {
			break
		}

		offset = offset + limit
	}

	return all, nil
}

func GetPlaylistTracks(spotifyClient *spotify.Client, id spotify.ID) ([]spotify.PlaylistTrack, error) {
	all := make([]spotify.PlaylistTrack, 0)
	limit := 100
	offset := 0
	options := spotify.Options{Limit: &limit, Offset: &offset}
	for {
		tracks, err := spotifyClient.GetPlaylistTracksOpt(id, &options, "")
		if err != nil {
			return nil, err
		}

		all = append(all, tracks.Tracks...)

		if len(tracks.Tracks) < *options.Limit {
			break
		}

		offset := *options.Limit + *options.Offset
		options.Offset = &offset
	}

	return all, nil
}

type TracksSet struct {
	Ids map[spotify.ID]bool
}

func NewTracksSet(ids []spotify.ID) (ts *TracksSet) {
	idsMap := make(map[spotify.ID]bool)

	for _, id := range ids {
		idsMap[id] = true
	}

	return &TracksSet{
		Ids: idsMap,
	}
}

func NewEmptyTracksSet() (ts *TracksSet) {
	idsMap := make(map[spotify.ID]bool)

	return &TracksSet{
		Ids: idsMap,
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

func RemoveTracksSetFromPlaylist(spotifyClient *spotify.Client, id spotify.ID, ts *TracksSet) (err error) {
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

func AddTracksSetToPlaylist(spotifyClient *spotify.Client, id spotify.ID, ts *TracksSet) (err error) {
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

func GetTrackIdsFromPlaylistTracks(tracks []spotify.PlaylistTrack) (ids []spotify.ID) {
	for _, track := range tracks {
		ids = append(ids, track.Track.ID)
	}

	return
}

func GetTrackIdsFromSimpleTracks(tracks []spotify.SimpleTrack) (ids []spotify.ID) {
	for _, track := range tracks {
		ids = append(ids, track.ID)
	}

	return
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

func MapIdsFromPlaylistTracks(tracks []spotify.PlaylistTrack) (ifaces []interface{}) {
	for _, t := range tracks {
		ifaces = append(ifaces, t.Track.ID)
	}
	return
}

func MapIdsFromSimpleTracks(tracks []spotify.SimpleTrack) (ifaces []interface{}) {
	for _, t := range tracks {
		ifaces = append(ifaces, t.ID)
	}
	return
}

func MapIds(ids []spotify.ID) (ifaces []interface{}) {
	for _, id := range ids {
		ifaces = append(ifaces, id)
	}
	return
}
