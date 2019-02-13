package main

import (
	"fmt"
	"log"
	"time"
	"strings"

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

		log.Printf("Created destination: %v", created)

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

func (pu *PlaylistUpdate) GetIdsToRemove() []spotify.ID {
	afterSet := mapset.NewSetFromSlice(MapIds(pu.idsAfter))
	idsToRemove := pu.idsBefore.Difference(afterSet)
	return ToSpotifyIds(idsToRemove.ToSlice())
}

func (pu *PlaylistUpdate) GetIdsToAdd() []spotify.ID {
	ids := make([]spotify.ID, 0)
	for _, id := range pu.idsAfter {
		if !pu.idsBefore.Contains(id) {
			ids = append(ids, id)
		}
	}
	return ids
}

func (pu *PlaylistUpdate) MergeBeforeAndToAdd() {
	for _, id := range pu.idsAfter {
		pu.idsBefore.Add(id)
	}
}
