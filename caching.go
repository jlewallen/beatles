package main

import (
	"encoding/json"
	"io/ioutil"
	"fmt"
	"os"
	"log"

	"github.com/zmb3/spotify"
)

type SpotifyCacher struct {
	spotifyClient *spotify.Client
}

func (sc *SpotifyCacher) GetPlaylists(user string) (playlists *PlaylistSet, err error) {
	cachedFile := fmt.Sprintf("playlists-%s.json", user)
	if _, err := os.Stat(cachedFile); !os.IsNotExist(err) {
		file, err := ioutil.ReadFile(cachedFile)
		if err != nil {
			return nil, err
		}

		playlists = &PlaylistSet{}
		err = json.Unmarshal(file, playlists)
		if err != nil {
			return nil, err
		}

		log.Printf("Returning cached %v", cachedFile)

		return playlists, nil
	}

	log.Printf("HI")

	limit := 50
	offset := 0
	options := spotify.Options{Limit: &limit, Offset: &offset}
	playlists = &PlaylistSet{
		Playlists: make([]Playlist, 0),
	}
	for {
		page, err := sc.spotifyClient.GetPlaylistsForUserOpt(user, &options)
		if err != nil {
			return nil, err
		}

		for _, iter := range page.Playlists {
			playlists.Playlists = append(playlists.Playlists, Playlist{
				ID:   iter.ID,
				Name: iter.Name,
				User: user,
			})
		}

		if len(page.Playlists) < *options.Limit {
			break
		}

		offset := *options.Limit + *options.Offset
		options.Offset = &offset
	}

	json, err := json.Marshal(playlists)
	if err != nil {
		return nil, fmt.Errorf("Error saving Playlists: %v", err)
	}

	err = ioutil.WriteFile(cachedFile, json, 0644)
	if err != nil {
		return nil, fmt.Errorf("Error saving Playlists: %v", err)
	}

	return
}

func (sc *SpotifyCacher) Invalidate(id spotify.ID) {
	cachedFile := fmt.Sprintf("playlist-%s.json", id)
	os.Remove(cachedFile)

	log.Printf("Invalidating playlist %v", id)
}

func (sc *SpotifyCacher) GetPlaylistTracks(userId string, id spotify.ID) (allTracks []spotify.PlaylistTrack, err error) {
	cachedFile := fmt.Sprintf("playlist-%s.json", id)
	if _, err := os.Stat(cachedFile); !os.IsNotExist(err) {
		file, err := ioutil.ReadFile(cachedFile)
		if err != nil {
			return nil, fmt.Errorf("Error opening %v", err)
		}

		allTracks = make([]spotify.PlaylistTrack, 0)
		err = json.Unmarshal(file, &allTracks)
		if err != nil {
			return nil, fmt.Errorf("Error unmarshalling %v", err)
		}

		log.Printf("Returning cached %s", cachedFile)

		return allTracks, nil
	}

	limit := 100
	offset := 0
	options := spotify.Options{Limit: &limit, Offset: &offset}
	for {
		tracks, spotifyErr := sc.spotifyClient.GetPlaylistTracksOpt(id, &options, "")
		if spotifyErr != nil {
			err = spotifyErr
			return
		}

		allTracks = append(allTracks, tracks.Tracks...)

		if len(tracks.Tracks) < *options.Limit {
			break
		}

		offset := *options.Limit + *options.Offset
		options.Offset = &offset
	}

	json, err := json.Marshal(allTracks)
	if err != nil {
		return nil, fmt.Errorf("Error saving playlist tracks: %v", err)
	}

	err = ioutil.WriteFile(cachedFile, json, 0644)
	if err != nil {
		return nil, fmt.Errorf("Error saving playlist tracks: %v", err)
	}

	return
}
