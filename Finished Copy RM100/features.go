package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type queueItem struct {
	URI     string `json:"uri"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
}

func visibleQueueItems(q *playbackQueue) []queueItem {
	items := make([]queueItem, 0, 10)
	if q == nil {
		return items
	}
	if q.CurrentlyPlaying != nil {
		current := *q.CurrentlyPlaying
		current.Type = "NOW PLAYING"
		items = append(items, current)
	}
	for _, item := range q.Queue {
		if len(items) >= 10 {
			break
		}
		items = append(items, item)
	}
	return items
}

func queueTrackAt(point touchPoint, landscape bool, q *playbackQueue) (queueItem, int, bool) {
	items := visibleQueueItems(q)
	if len(items) == 0 {
		return queueItem{}, -1, false
	}
	r := queuePanelRect(landscape)
	startY := int(r.top) + 116
	rowHeight := (int(r.height) - 138) / len(items)
	if rowHeight > 104 {
		rowHeight = 104
	}
	row := (point.y - startY) / rowHeight
	if point.y < startY || row < 0 || row >= len(items) {
		return queueItem{}, -1, false
	}
	// The first row is the current track when one is available, so only
	// upcoming queue entries can be selected.
	if q != nil && q.CurrentlyPlaying != nil {
		row--
	}
	if q == nil || row < 0 || row >= len(q.Queue) || q.Queue[row].URI == "" {
		return queueItem{}, -1, false
	}
	return q.Queue[row], row, true
}

type playbackQueue struct {
	CurrentlyPlaying *queueItem  `json:"currently_playing"`
	Queue            []queueItem `json:"queue"`
}

type networkInfo struct {
	SSID      string
	IPv4      string
	Connected bool
}

func (s *spotify) getQueue() (*playbackQueue, error) {
	if err := s.ensureAccessToken(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, apiBase+"/me/player/queue", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token.AccessToken)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
		return nil, fmt.Errorf("Spotify queue returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result playbackQueue
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *spotify) advanceToQueueIndex(index int) error {
	if index < 0 {
		return fmt.Errorf("invalid queue selection")
	}
	// Advance through Spotify's current queue instead of starting an isolated
	// track URI, which preserves the playlist/queue context after the selection.
	for step := 0; step <= index; step++ {
		if err := s.playbackCommand("next"); err != nil {
			return fmt.Errorf("could not advance to queue item %d: %w", index+1, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func (s *spotify) setShuffle(enabled bool) error {
	params := url.Values{"state": {fmt.Sprintf("%t", enabled)}}
	return s.playerSetting("/me/player/shuffle?" + params.Encode())
}

func (s *spotify) setRepeat(mode string) error {
	params := url.Values{"state": {mode}}
	return s.playerSetting("/me/player/repeat?" + params.Encode())
}

func (s *spotify) playerSetting(path string) error {
	if err := s.ensureAccessToken(); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, apiBase+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token.AccessToken)
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
	return fmt.Errorf("Spotify playback setting returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func (s *spotify) trackSaved(uri string) (bool, error) {
	if err := s.ensureAccessToken(); err != nil {
		return false, err
	}
	params := url.Values{"uris": {uri}}
	req, err := http.NewRequest(http.MethodGet, apiBase+"/me/library/contains?"+params.Encode(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token.AccessToken)
	resp, err := s.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
		return false, fmt.Errorf("Spotify library check returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var saved []bool
	if err := json.NewDecoder(resp.Body).Decode(&saved); err != nil {
		return false, err
	}
	return len(saved) > 0 && saved[0], nil
}

func (s *spotify) saveTrack(uri string) error {
	if err := s.ensureAccessToken(); err != nil {
		return err
	}
	params := url.Values{"uris": {uri}}
	req, err := http.NewRequest(http.MethodPut, apiBase+"/me/library?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token.AccessToken)
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
	return fmt.Errorf("Spotify could not save this track (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func readNetworkInfo() networkInfo {
	info := networkInfo{}
	if iface, err := net.InterfaceByName("wlan0"); err == nil && iface.Flags&net.FlagUp != 0 {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.To4() != nil {
				info.IPv4 = ip.String()
				info.Connected = true
				break
			}
		}
	}
	if ssid, connected := queryWiFiSSID(); connected {
		info.SSID = ssid
		info.Connected = true
	}
	return info
}

func queryWiFiSSID() (string, bool) {
	remote := &net.UnixAddr{Name: "/var/run/wpa_supplicant/wlan0", Net: "unixgram"}
	local := &net.UnixAddr{Name: filepath.Join(os.TempDir(), fmt.Sprintf("rm100-wpa-%d", os.Getpid())), Net: "unixgram"}
	conn, err := net.DialUnix("unixgram", local, remote)
	if err != nil {
		return "", false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write([]byte("STATUS")); err != nil {
		return "", false
	}
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		return "", false
	}
	status := string(buf[:n])
	completed := false
	ssid := ""
	for _, line := range strings.Split(status, "\n") {
		if line == "wpa_state=COMPLETED" {
			completed = true
		}
		if strings.HasPrefix(line, "ssid=") {
			ssid = strings.TrimSpace(strings.TrimPrefix(line, "ssid="))
		}
	}
	return ssid, completed
}
