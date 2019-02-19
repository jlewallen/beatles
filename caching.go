package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/zmb3/spotify"
)

const VerboseLogging = false

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

		if VerboseLogging {
			log.Printf("Returning cached %v", cachedFile)
		}

		return playlists, nil
	}

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

		if VerboseLogging {
			log.Printf("Returning cached %s", cachedFile)
		}

		return allTracks, nil
	}

	allTracks, spotifyErr := GetPlaylistTracks(sc.spotifyClient, id)
	if spotifyErr != nil {
		err = spotifyErr
		return
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

func (sc *SpotifyCacher) GetAlbum(id spotify.ID) (album *spotify.FullAlbum, err error) {
	cachedFile := fmt.Sprintf("album-%s.json", id)
	if _, err := os.Stat(cachedFile); !os.IsNotExist(err) {
		file, err := ioutil.ReadFile(cachedFile)
		if err != nil {
			return nil, fmt.Errorf("Error opening %v", err)
		}

		var album *spotify.FullAlbum
		err = json.Unmarshal(file, &album)
		if err != nil {
			return nil, fmt.Errorf("Error unmarshalling %v", err)
		}

		if VerboseLogging {
			log.Printf("Returning cached %s", cachedFile)
		}

		return album, nil
	}

	album, spotifyErr := sc.spotifyClient.GetAlbum(id)
	if spotifyErr != nil {
		err = spotifyErr
		return
	}

	json, err := json.Marshal(album)
	if err != nil {
		return nil, fmt.Errorf("Error saving album tracks: %v", err)
	}

	err = ioutil.WriteFile(cachedFile, json, 0644)
	if err != nil {
		return nil, fmt.Errorf("Error saving album tracks: %v", err)
	}

	return
}

func (sc *SpotifyCacher) GetAlbumTracks(id spotify.ID) (allTracks []spotify.SimpleTrack, err error) {
	cachedFile := fmt.Sprintf("album-tracks-%s.json", id)
	if _, err := os.Stat(cachedFile); !os.IsNotExist(err) {
		file, err := ioutil.ReadFile(cachedFile)
		if err != nil {
			return nil, fmt.Errorf("Error opening %v", err)
		}

		allTracks := make([]spotify.SimpleTrack, 0)
		err = json.Unmarshal(file, &allTracks)
		if err != nil {
			return nil, fmt.Errorf("Error unmarshalling %v", err)
		}

		if VerboseLogging {
			log.Printf("Returning cached %s", cachedFile)
		}

		return allTracks, nil
	}

	allTracks, spotifyErr := GetAlbumTracks(sc.spotifyClient, id)
	if spotifyErr != nil {
		err = spotifyErr
		return
	}

	json, err := json.Marshal(allTracks)
	if err != nil {
		return nil, fmt.Errorf("Error saving album tracks: %v", err)
	}

	err = ioutil.WriteFile(cachedFile, json, 0644)
	if err != nil {
		return nil, fmt.Errorf("Error saving album tracks: %v", err)
	}

	return
}

func (sc *SpotifyCacher) GetArtistAlbums(id spotify.ID) (allAlbums []spotify.SimpleAlbum, err error) {
	cachedFile := fmt.Sprintf("artist-albums-%s.json", id)
	if _, err := os.Stat(cachedFile); !os.IsNotExist(err) {
		file, err := ioutil.ReadFile(cachedFile)
		if err != nil {
			return nil, fmt.Errorf("Error opening %v", err)
		}

		allAlbums := make([]spotify.SimpleAlbum, 0)
		err = json.Unmarshal(file, &allAlbums)
		if err != nil {
			return nil, fmt.Errorf("Error unmarshalling %v", err)
		}

		if VerboseLogging {
			log.Printf("Returning cached %s", cachedFile)
		}

		return allAlbums, nil
	}

	allAlbums, spotifyErr := GetArtistAlbums(sc.spotifyClient, id)
	if spotifyErr != nil {
		err = spotifyErr
		return
	}

	json, err := json.Marshal(allAlbums)
	if err != nil {
		return nil, fmt.Errorf("Error saving artist albums: %v", err)
	}

	err = ioutil.WriteFile(cachedFile, json, 0644)
	if err != nil {
		return nil, fmt.Errorf("Error saving artist albums: %v", err)
	}

	return
}

func (sc *SpotifyCacher) GetTracks(ids []spotify.ID) (tracks []spotify.FullTrack, err error) {
	requesting := make([]spotify.ID, 0)
	for _, id := range ids {
		cachedFile := fmt.Sprintf("track-%s.json", id)
		if _, err := os.Stat(cachedFile); os.IsNotExist(err) {
			requesting = append(requesting, id)
		}
	}

	if len(requesting) > 0 {
		requested, spotifyErr := sc.spotifyClient.GetTracks(requesting...)
		if spotifyErr != nil {
			err = spotifyErr
			return
		}

		for _, track := range requested {
			cachedFile := fmt.Sprintf("track-%s.json", track.ID)

			json, err := json.Marshal(track)
			if err != nil {
				return nil, fmt.Errorf("Error saving track: %v", err)
			}

			err = ioutil.WriteFile(cachedFile, json, 0644)
			if err != nil {
				return nil, fmt.Errorf("Error saving track: %v", err)
			}
		}
	}

	tracks = make([]spotify.FullTrack, 0)

	for _, id := range ids {
		cachedFile := fmt.Sprintf("track-%s.json", id)
		if _, err := os.Stat(cachedFile); !os.IsNotExist(err) {
			file, err := ioutil.ReadFile(cachedFile)
			if err != nil {
				return nil, fmt.Errorf("Error opening %v", err)
			}

			var track spotify.FullTrack
			err = json.Unmarshal(file, &track)
			if err != nil {
				return nil, fmt.Errorf("Error unmarshalling %v", err)
			}

			tracks = append(tracks, track)

			if VerboseLogging {
				log.Printf("Returning cached %s", cachedFile)
			}
		}
	}

	return
}
