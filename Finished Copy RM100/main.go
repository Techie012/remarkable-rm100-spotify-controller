package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	clientID    = "9ddfe23f0c36416f8b0e8b28a7b1b797"
	redirectURI = "http://127.0.0.1:8080/callback"
	authURL     = "https://accounts.spotify.com/authorize"
	tokenURL    = "https://accounts.spotify.com/api/token"
	apiBase     = "https://api.spotify.com/v1"
	tokenScope  = "user-read-currently-playing user-read-playback-state user-modify-playback-state user-library-read user-library-modify"
)

type token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

type playback struct {
	IsPlaying  bool            `json:"is_playing"`
	ProgressMS int64           `json:"progress_ms"`
	Shuffle    bool            `json:"shuffle_state"`
	Repeat     string          `json:"repeat_state"`
	Device     *playbackDevice `json:"device"`
	Item       *struct {
		ID         string `json:"id"`
		URI        string `json:"uri"`
		Name       string `json:"name"`
		DurationMS int64  `json:"duration_ms"`
		Artists    []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Album struct {
			Name   string `json:"name"`
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"album"`
	} `json:"item"`
}

type playbackDevice struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	IsActive       bool   `json:"is_active"`
	VolumePercent  *int   `json:"volume_percent"`
	SupportsVolume bool   `json:"supports_volume"`
}

type spotifyDevice struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	IsActive       bool   `json:"is_active"`
	IsRestricted   bool   `json:"is_restricted"`
	VolumePercent  *int   `json:"volume_percent"`
	SupportsVolume bool   `json:"supports_volume"`
}

type spotify struct {
	http  *http.Client
	token token
}

type rateLimitError struct {
	retryAfter time.Duration
}

func (e *rateLimitError) Error() string {
	return fmt.Sprintf("rate limited; retrying in %s", e.retryAfter)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "auth" {
		if err := authorize(); err != nil {
			fmt.Fprintln(os.Stderr, "Spotify sign-in failed:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "RM100 Spotify player:", err)
		os.Exit(1)
	}
}

func tokenPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/home/root/.rm100-spotify-token.json"
	}
	return filepath.Join(home, ".rm100-spotify-token.json")
}

func loadToken() (token, error) {
	var t token
	data, err := os.ReadFile(tokenPath())
	if err != nil {
		return t, fmt.Errorf("not signed in; run ./rm100_spotify auth first")
	}
	err = json.Unmarshal(data, &t)
	return t, err
}

func saveToken(t token) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	path := tokenPath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func randomText(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func authorize() error {
	verifier, err := randomText(64)
	if err != nil {
		return err
	}
	state, err := randomText(24)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {tokenScope},
		"state":                 {state},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
	}
	result := make(chan url.Values, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "Sign-in state did not match. Please retry.", http.StatusBadRequest)
			return
		}
		select {
		case result <- q:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintln(w, "<html><body><h2>Spotify sign-in complete</h2><p>Close this tab and return to the RM100 terminal.</p></body></html>")
		default:
			http.Error(w, "This sign-in has already completed.", http.StatusGone)
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		return fmt.Errorf("cannot open callback port 8080: %w", err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go server.Serve(listener)

	fmt.Println("On Windows, open an SSH tunnel in a second PowerShell window:")
	fmt.Println("  ssh -N -L 8080:127.0.0.1:8080 root@192.168.1.12")
	fmt.Println("Keep that window open. Then open this sign-in link in your Windows browser:")
	fmt.Println(authURL + "?" + params.Encode())
	fmt.Println("Waiting up to 3 minutes for Spotify authorization...")
	select {
	case q := <-result:
		if q.Get("error") != "" {
			return fmt.Errorf("Spotify returned %s", q.Get("error"))
		}
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {q.Get("code")},
			"redirect_uri":  {redirectURI},
			"client_id":     {clientID},
			"code_verifier": {verifier},
		}
		resp, err := http.PostForm(tokenURL, form)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return fmt.Errorf("token exchange returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var got struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int64  `json:"expires_in"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			return err
		}
		if got.RefreshToken == "" || got.AccessToken == "" {
			return errors.New("Spotify did not return the expected tokens")
		}
		if err := saveToken(token{AccessToken: got.AccessToken, RefreshToken: got.RefreshToken, ExpiresAt: time.Now().Unix() + got.ExpiresIn}); err != nil {
			return err
		}
		fmt.Println("Spotify sign-in saved on the RM100.")
		return nil
	case <-time.After(3 * time.Minute):
		return errors.New("timed out waiting for the browser callback")
	}
}

func newSpotify(t token) *spotify {
	return &spotify{http: &http.Client{Timeout: 20 * time.Second}, token: t}
}

func (s *spotify) refresh() error {
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {s.token.RefreshToken}, "client_id": {clientID}}
	resp, err := s.http.PostForm(tokenURL, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh returned HTTP %d", resp.StatusCode)
	}
	var got struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		return err
	}
	s.token.AccessToken = got.AccessToken
	if got.RefreshToken != "" {
		s.token.RefreshToken = got.RefreshToken
	}
	s.token.ExpiresAt = time.Now().Unix() + got.ExpiresIn
	return saveToken(s.token)
}

func (s *spotify) getPlayback() (*playback, error) {
	if err := s.ensureAccessToken(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, apiBase+"/me/player", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token.AccessToken)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
		if resp.StatusCode == http.StatusTooManyRequests {
			retrySeconds, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			if retrySeconds <= 0 {
				retrySeconds = 30
			}
			return nil, &rateLimitError{retryAfter: time.Duration(retrySeconds) * time.Second}
		}
		return nil, fmt.Errorf("playback request returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var p playback
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *spotify) ensureAccessToken() error {
	if time.Now().Unix() > s.token.ExpiresAt-60 {
		return s.refresh()
	}
	return nil
}

func (s *spotify) availableDevices() ([]spotifyDevice, error) {
	if err := s.ensureAccessToken(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, apiBase+"/me/player/devices", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token.AccessToken)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
		return nil, fmt.Errorf("Spotify device list returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Devices []spotifyDevice `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	available := result.Devices[:0]
	for _, device := range result.Devices {
		if device.ID != "" && !device.IsRestricted {
			available = append(available, device)
		}
	}
	return available, nil
}

func (s *spotify) transferPlayback(deviceID string) error {
	if err := s.ensureAccessToken(); err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		DeviceIDs []string `json:"device_ids"`
	}{DeviceIDs: []string{deviceID}})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, apiBase+"/me/player", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
	return fmt.Errorf("Spotify could not transfer playback (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
}

func (s *spotify) playbackCommand(command string) error {
	if err := s.ensureAccessToken(); err != nil {
		return err
	}
	var endpoint string
	var method = http.MethodPut
	switch command {
	case "play", "pause":
		endpoint = apiBase + "/me/player/" + command
	case "next":
		endpoint = apiBase + "/me/player/next"
		method = http.MethodPost
	case "previous":
		endpoint = apiBase + "/me/player/previous"
		method = http.MethodPost
	case "volume_up", "volume_down":
		return fmt.Errorf("%s needs a target volume", command)
	default:
		return fmt.Errorf("unknown playback command %q", command)
	}
	req, err := http.NewRequest(method, endpoint, nil)
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
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("Spotify denied the command; run the auth command again to grant playback-control permission (HTTP 403)")
	}
	return fmt.Errorf("Spotify %s command returned HTTP %d: %s", command, resp.StatusCode, strings.TrimSpace(string(body)))
}

func (s *spotify) setVolume(percent int) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	params := url.Values{"volume_percent": {strconv.Itoa(percent)}}
	req, err := http.NewRequest(http.MethodPut, apiBase+"/me/player/volume?"+params.Encode(), nil)
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
	if resp.StatusCode == http.StatusForbidden {
		return errors.New("Spotify denied volume control; it requires Premium and a volume-capable active device")
	}
	return fmt.Errorf("Spotify volume command returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
func (s *spotify) image(coverURL string) *image.Image {
	if coverURL == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil
	}
	var result image.Image = img
	return &result
}

func run() error {
	t, err := loadToken()
	if err != nil {
		return err
	}
	if err := exec.Command("systemctl", "stop", "xochitl").Run(); err != nil {
		return fmt.Errorf("could not stop xochitl: %w (restore it with systemctl start xochitl)", err)
	}
	defer exec.Command("systemctl", "start", "xochitl").Run()

	fb, err := openFramebuffer()
	if err != nil {
		return err
	}
	defer fb.close()
	s := newSpotify(t)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lastKey, lastPlayState := "", false
	var deviceList []spotifyDevice
	deviceMenuOpen, deviceMenuOffset := false, 0
	var currentQueue *playbackQueue
	queueOpen, networkOpen := false, false
	var networkState networkInfo
	var cover *image.Image
	touches := startTouchReader()
	if touches == nil {
		fmt.Println("No touchscreen input device was found; touchscreen controls will not respond.")
	}
	fmt.Println("RM100 Spotify display running. Spotify state updates every second; touch PAPER UI or press Ctrl-C to return to the normal reMarkable screen.")
	var current *playback
	handlePlayback := func(p *playback) {
		current = p
		key := playbackKey(p)
		isPlaying := p != nil && p.IsPlaying
		if fb.lowBattery {
			lastPlayState = isPlaying
			return
		}
		if key != lastKey {
			fb.trackSaved = false
			fb.notice = ""
			if p != nil && p.Item != nil && len(p.Item.Album.Images) > 0 {
				cover = s.image(p.Item.Album.Images[0].URL)
			} else {
				cover = nil
			}
			if p != nil && p.Item != nil && p.Item.URI != "" {
				saved, saveErr := s.trackSaved(p.Item.URI)
				if saveErr != nil {
					fmt.Println("Spotify library:", saveErr)
					if strings.Contains(saveErr.Error(), "HTTP 403") {
						fb.notice = "AUTHORIZE LIBRARY ACCESS TO SAVE TRACKS"
					}
				} else {
					fb.trackSaved = saved
				}
			}
			if err := fb.drawStatic(p, cover); err != nil {
				fmt.Println("Display:", err)
			} else {
				lastKey = key
				fmt.Println("Now playing:", trackTitle(p))
				if deviceMenuOpen {
					fb.drawDeviceMenu(deviceList, deviceMenuOffset)
					_ = fb.refresh(deviceMenuRect(fb.landscape, len(deviceList)), waveGC16, updatePartial)
				}
				if queueOpen {
					fb.drawQueueOverlay(currentQueue)
					_ = fb.refresh(queuePanelRect(fb.landscape), waveGC16, updatePartial)
				}
				if networkOpen {
					networkState = readNetworkInfo()
					fb.drawNetworkOverlay(networkState)
					_ = fb.refresh(networkPanelRect(fb.landscape), waveGC16, updatePartial)
				}
			}
		} else if isPlaying != lastPlayState {
			fb.drawPlayIcon(isPlaying)
			if err := fb.refresh(fb.controlRefreshRect(), waveGC16, updatePartial); err != nil {
				fmt.Println("Display:", err)
			}
		}
		lastPlayState = isPlaying
	}
	toggleOrientation := func() {
		deviceMenuOpen = false
		queueOpen, networkOpen = false, false
		fb.landscape = !fb.landscape
		if err := fb.drawStatic(current, cover); err != nil {
			fmt.Println("Display:", err)
			fb.landscape = !fb.landscape
			return
		}
		layout := "portrait"
		if fb.landscape {
			layout = "landscape"
		}
		fmt.Println("Display orientation:", layout)
	}
	pollDelay := time.Second
	pollPlayback := func() {
		p, err := s.getPlayback()
		if err != nil {
			var limited *rateLimitError
			if errors.As(err, &limited) {
				fmt.Println("Spotify:", err, "(following Spotify Retry-After guidance)")
				pollDelay = limited.retryAfter
			} else {
				fmt.Println("Spotify:", err)
				pollDelay = time.Second
			}
			return
		}
		pollDelay = time.Second
		handlePlayback(p)
		if p != nil && p.Item != nil {
			fmt.Printf("Spotify state: playing=%t\n", p.IsPlaying)
		} else {
			fmt.Println("Spotify state: no active playback")
		}
	}
	showError := func(prefix string, err error) {
		fmt.Println(prefix, err)
		if fb.notice == "" {
			fb.notice = "SPOTIFY ACTION FAILED"
		}
		_ = fb.drawStatic(current, cover)
	}
	performControl := func(action string) {
		if action == "rotate" {
			toggleOrientation()
			return
		}
		if action == "exit" {
			fmt.Println("Touch: returning to the normal reMarkable interface")
			stop()
			return
		}
		if action == "shuffle" {
			enabled := current == nil || !current.Shuffle
			if err := s.setShuffle(enabled); err != nil {
				showError("Spotify shuffle:", err)
				return
			}
			if current != nil {
				current.Shuffle = enabled
			}
			_ = fb.drawStatic(current, cover)
			return
		}
		if action == "repeat" {
			mode := "context"
			if current != nil {
				switch current.Repeat {
				case "context":
					mode = "track"
				case "track":
					mode = "off"
				}
			}
			if err := s.setRepeat(mode); err != nil {
				showError("Spotify repeat:", err)
				return
			}
			if current != nil {
				current.Repeat = mode
			}
			_ = fb.drawStatic(current, cover)
			return
		}
		if action == "save_track" {
			if current == nil || current.Item == nil || current.Item.URI == "" || fb.trackSaved {
				return
			}
			if err := s.saveTrack(current.Item.URI); err != nil {
				if strings.Contains(err.Error(), "HTTP 403") {
					fb.notice = "RUN SPOTIFY AUTH TO ENABLE TRACK SAVING"
				}
				showError("Spotify save:", err)
				return
			}
			fb.trackSaved = true
			fb.notice = "TRACK SAVED TO YOUR LIBRARY"
			_ = fb.drawStatic(current, cover)
			return
		}
		if action == "volume_up" || action == "volume_down" {
			if current == nil || current.Device == nil || current.Device.VolumePercent == nil || !current.Device.SupportsVolume {
				fmt.Println("Spotify volume: the active device does not report controllable volume")
				return
			}
			target := *current.Device.VolumePercent
			if action == "volume_up" {
				target += 5
			} else {
				target -= 5
			}
			if target < 0 {
				target = 0
			}
			if target > 100 {
				target = 100
			}
			if err := s.setVolume(target); err != nil {
				fmt.Println("Spotify volume:", err)
				return
			}
			*current.Device.VolumePercent = target
			fmt.Printf("Spotify volume set to %d%%\n", target)
			return
		}
		command := action
		if action == "play_pause" {
			command = "play"
			if current != nil && current.Item != nil && current.IsPlaying {
				command = "pause"
			}
		}
		fmt.Printf("Touch: %s button pressed\n", action)
		if err := s.playbackCommand(command); err != nil {
			fmt.Println("Spotify control:", err)
			return
		}
		fmt.Println("Spotify control:", command, "sent")
	}
	pollPlayback()
	pollTimer := time.NewTimer(pollDelay)
	defer pollTimer.Stop()
	batteryTicker := time.NewTicker(10 * time.Second)
	defer batteryTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-pollTimer.C:
			pollPlayback()
			pollTimer.Reset(pollDelay)
		case <-batteryTicker.C:
			changed, err := fb.refreshBattery()
			if err != nil {
				fmt.Println("Battery display:", err)
			}
			if changed {
				if fb.lowBattery {
					deviceMenuOpen, queueOpen, networkOpen = false, false, false
				}
				if err := fb.drawStatic(current, cover); err != nil {
					fmt.Println("Display:", err)
				}
			}
		case point, ok := <-touches:
			if ok {
				if fb.lowBattery {
					continue
				}
				logical := fb.logicalTouch(point)
				if networkOpen {
					if pointInRect(logical, networkButtonRect()) {
						networkOpen = false
						_ = fb.drawStatic(current, cover)
						continue
					}
					if pointInRect(logical, networkPanelRect(fb.landscape)) {
						networkOpen = false
						_ = fb.drawStatic(current, cover)
						continue
					}
					networkOpen = false
					_ = fb.drawStatic(current, cover)
				}
				if queueOpen {
					actionIndex := featureButtonAt(logical, fb.landscape)
					if pointInRect(logical, queueCloseRect(fb.landscape)) || actionIndex == 0 || !pointInRect(logical, queuePanelRect(fb.landscape)) {
						queueOpen = false
						_ = fb.drawStatic(current, cover)
						if pointInRect(logical, queueCloseRect(fb.landscape)) || actionIndex == 0 {
							continue
						}
					} else {
						if item, index, ok := queueTrackAt(logical, fb.landscape, currentQueue); ok {
							queueOpen = false
							if err := s.advanceToQueueIndex(index); err != nil {
								fmt.Println("Spotify queue selection:", err)
								fb.notice = "COULD NOT PLAY TRACK"
								_ = fb.drawStatic(current, cover)
								continue
							}
							fmt.Println("Skipping to queued track:", item.Name)
							_ = fb.drawStatic(current, cover)
							pollPlayback()
						}
						continue
					}
				}
				if deviceMenuOpen {
					menuRect := deviceMenuRect(fb.landscape, len(deviceList))
					if pointInRect(logical, menuRect) {
						index := deviceMenuSelection(logical, fb.landscape, deviceMenuOffset, len(deviceList))
						if index == -2 {
							if deviceMenuOffset+4 < len(deviceList) {
								deviceMenuOffset += 4
							} else {
								deviceMenuOffset = 0
							}
							fb.drawDeviceMenu(deviceList, deviceMenuOffset)
							_ = fb.refresh(menuRect, waveGC16, updatePartial)
							continue
						}
						if index >= 0 && index < len(deviceList) {
							device := deviceList[index]
							deviceMenuOpen = false
							if err := s.transferPlayback(device.ID); err != nil {
								fmt.Println("Spotify device:", err)
								_ = fb.drawStatic(current, cover)
								continue
							}
							if current != nil {
								current.Device = &playbackDevice{
									ID: device.ID, Name: device.Name, IsActive: true,
									VolumePercent: device.VolumePercent, SupportsVolume: device.SupportsVolume,
								}
								handlePlayback(current)
							} else {
								_ = fb.drawStatic(nil, nil)
							}
							fmt.Println("Spotify playback transferred to:", device.Name)
							continue
						}
						continue
					}
					deviceMenuOpen = false
					_ = fb.drawStatic(current, cover)
					if pointInRect(logical, deviceSelectorRect(fb.landscape)) {
						continue
					}
				}
				action := hitTestControl(logical, fb.landscape)
				fmt.Printf("Touch: screen=%d,%d logical=%d,%d action=%s\n", point.x, point.y, logical.x, logical.y, action)
				if action == "device_selector" {
					queueOpen, networkOpen = false, false
					devices, err := s.availableDevices()
					if err != nil {
						fmt.Println("Spotify devices:", err)
						deviceList = nil
					} else {
						deviceList = devices
					}
					deviceMenuOffset = 0
					deviceMenuOpen = true
					fb.drawDeviceMenu(deviceList, deviceMenuOffset)
					if err := fb.refresh(deviceMenuRect(fb.landscape, len(deviceList)), waveGC16, updatePartial); err != nil {
						fmt.Println("Display:", err)
					}
					continue
				}
				if action == "queue" {
					deviceMenuOpen, networkOpen = false, false
					q, err := s.getQueue()
					if err != nil {
						fmt.Println("Spotify queue:", err)
						fb.notice = "QUEUE UNAVAILABLE"
						q = nil
					}
					currentQueue = q
					queueOpen = true
					fb.drawQueueOverlay(currentQueue)
					if err := fb.refresh(queuePanelRect(fb.landscape), waveGC16, updatePartial); err != nil {
						fmt.Println("Display:", err)
					}
					continue
				}
				if action == "wifi_info" {
					deviceMenuOpen, queueOpen = false, false
					networkOpen = true
					networkState = readNetworkInfo()
					fb.drawNetworkOverlay(networkState)
					if err := fb.refresh(networkPanelRect(fb.landscape), waveGC16, updatePartial); err != nil {
						fmt.Println("Display:", err)
					}
					continue
				}
				if action == "refresh" {
					pollPlayback()
					changed, err := fb.refreshBattery()
					if err != nil {
						fmt.Println("Battery display:", err)
					}
					if changed {
						if fb.lowBattery {
							deviceMenuOpen, queueOpen, networkOpen = false, false, false
						}
						_ = fb.drawStatic(current, cover)
						if fb.lowBattery {
							continue
						}
					}
					if deviceMenuOpen {
						if devices, err := s.availableDevices(); err == nil {
							deviceList = devices
							deviceMenuOffset = 0
							fb.drawDeviceMenu(deviceList, deviceMenuOffset)
							_ = fb.refresh(deviceMenuRect(fb.landscape, len(deviceList)), waveGC16, updatePartial)
						}
					}
					if networkOpen {
						networkState = readNetworkInfo()
						fb.drawNetworkOverlay(networkState)
						_ = fb.refresh(networkPanelRect(fb.landscape), waveGC16, updatePartial)
					}
					if queueOpen {
						if q, err := s.getQueue(); err == nil {
							currentQueue = q
							fb.drawQueueOverlay(currentQueue)
							_ = fb.refresh(queuePanelRect(fb.landscape), waveGC16, updatePartial)
						}
					}
					continue
				}
				if action != "" {
					performControl(action)
				}
			}
		}
	}
}

func hitTestControl(p touchPoint, landscape bool) string {
	if pointInRect(p, deviceSelectorRect(landscape)) {
		return "device_selector"
	}
	if pointInRect(p, networkButtonRect()) {
		return "wifi_info"
	}
	if pointInRect(p, refreshButtonRect()) {
		return "refresh"
	}
	if index := featureButtonAt(p, landscape); index >= 0 {
		return [...]string{"queue", "shuffle", "repeat", "save_track"}[index]
	}
	width := fbWidth
	if landscape {
		width = fbHeight
	}
	if p.y >= 30 && p.y <= 135 && p.x >= 60 && p.x <= 292 {
		return "exit"
	}
	if p.y >= 30 && p.y <= 135 && p.x >= width-390 && p.x <= width-125 {
		return "rotate"
	}
	if !landscape {
		if p.y >= 1200 && p.y <= 1340 {
			if p.x >= 420 && p.x <= 590 {
				return "volume_down"
			}
			if p.x >= 810 && p.x <= 990 {
				return "volume_up"
			}
			return ""
		}
		if p.y < 1350 || p.y > 1580 {
			return ""
		}
		if p.x >= 420 && p.x <= 590 {
			return "previous"
		}
		if p.x >= 590 && p.x <= 810 {
			return "play_pause"
		}
		if p.x >= 810 && p.x <= 990 {
			return "next"
		}
		return ""
	}
	if p.y >= 925 && p.y <= 1155 {
		if p.x >= 65 && p.x <= 205 {
			return "volume_down"
		}
		if p.x >= 710 && p.x <= 860 {
			return "volume_up"
		}
	}
	if p.y < 925 || p.y > 1155 {
		return ""
	}
	if p.x >= 1030 && p.x <= 1230 {
		return "previous"
	}
	if p.x >= 1230 && p.x <= 1480 {
		return "play_pause"
	}
	if p.x >= 1480 && p.x <= 1690 {
		return "next"
	}
	return ""
}

func pointInRect(p touchPoint, r rect) bool {
	return p.x >= int(r.left) && p.x < int(r.left+r.width) && p.y >= int(r.top) && p.y < int(r.top+r.height)
}

func deviceMenuSelection(p touchPoint, landscape bool, offset, count int) int {
	r := deviceMenuRect(landscape, count)
	rows := deviceMenuRowCount(count)
	rowHeight := int(r.height) / rows
	row := (p.y - int(r.top)) / rowHeight
	if row < 0 || row >= rows {
		return -1
	}
	if row == 4 && count > 4 {
		return -2
	}
	index := offset + row
	if row < 4 && index < count {
		return index
	}
	return -1
}

func playbackKey(p *playback) string {
	if p == nil || p.Item == nil {
		if p != nil && p.Device != nil {
			return "idle|" + p.Device.ID + "|" + p.Device.Name
		}
		return "idle"
	}
	var artists []string
	for _, a := range p.Item.Artists {
		artists = append(artists, a.Name)
	}
	device := ""
	if p.Device != nil {
		device = p.Device.ID + "|" + p.Device.Name
	}
	return p.Item.ID + "|" + p.Item.Name + "|" + strings.Join(artists, ",") + "|" + p.Item.Album.Name + "|" + device + fmt.Sprintf("|%t|%s", p.Shuffle, p.Repeat)
}

func trackTitle(p *playback) string {
	if p == nil || p.Item == nil {
		return "Nothing playing"
	}
	return p.Item.Name
}
