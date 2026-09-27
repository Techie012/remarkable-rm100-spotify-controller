# RM100 Spotify display

A native Spotify now-playing display for the original reMarkable tablet (RM100 / reMarkable 1). It runs directly on the tablet, uses the finger touchscreen, and can start automatically at boot.

This folder contains the Go source, a prebuilt Linux ARMv7 executable, the systemd startup service, and this installation guide. It does not contain anyone's Spotify login token, SSH key, Wi-Fi details, or device password.

## Features

- Album artwork, track, artist, and album details
- Previous, play/pause, next, volume, queue preview, shuffle, and repeat controls
- Save the current track to your Spotify library
- Portrait/landscape rotate button
- Battery percentage, charging bolt, Wi-Fi/IP panel, and refresh button
- EXIT SPOTIFY button to exit back to the normal reMarkable interface
- Automatic Spotify display launch after reboot
- Automatic idle sleep is blocked while Spotify mode is running; Paper UI sleep behavior resumes after exiting

The progress bar and track-time labels are intentionally omitted. Volume control requires a Spotify Premium account and an active playback device that supports volume.

## Requirements

- Original reMarkable 1 / RM100 with SSH access enabled. This version was developed on reMarkable OS `3.17.0.72`; other models and OS versions have not been confirmed.
- Wi-Fi with internet access for Spotify, and the tablet's SSH address and root password or an SSH key.
- A Spotify account and a Spotify Developer app.
- Windows 10/11 with OpenSSH Client for the commands below. Go 1.21 or later is only needed if you change the Spotify Client ID or rebuild the source.

## 1. Set up a Spotify Developer app

Create an app in the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard). Add this exact Redirect URI in the app settings:

    http://127.0.0.1:8080/callback

Copy the app's **Client ID**. In `main.go`, replace the value of `clientID` with your own Client ID. Do not put a Client Secret in the code; this app uses Authorization Code with PKCE and does not need one. Spotify documents PKCE as the flow for apps that cannot safely keep a client secret, and requires the redirect URI to match the allowlist exactly. [Create an app](https://developer.spotify.com/documentation/web-api/concepts/apps) · [Redirect URI rules](https://developer.spotify.com/documentation/web-api/concepts/redirect_uri) · [PKCE flow](https://developer.spotify.com/documentation/web-api/tutorials/code-pkce-flow)

Version 24 adds library saving controls and therefore requests the Spotify library read and modify permissions. Upgrading from an earlier build requires one new Spotify authorization; see the upgrade note in section 3.

If you changed `main.go`, double-click `build_rm100.bat` to rebuild. The result will be `dist\rm100_spotify_v24`. The included binary is already built, but it uses the Client ID embedded in this copy; rebuild after replacing that ID if you use your own Spotify app.

## 2. Copy the files to the tablet

Find the RM100's current Wi-Fi IP address in its normal interface or router. Open PowerShell in this folder and replace the sample address below with that IP:

    $Device = "root@192.168.1.XX"
    scp .\dist\rm100_spotify_v24 "${Device}:/tmp/rm100_spotify"
    scp .\rm100-spotify.service "${Device}:/tmp/rm100-spotify.service"
    scp .\rm100-spotify-start.timer "${Device}:/tmp/rm100-spotify-start.timer"
    ssh $Device

Enter the tablet's root password if asked. The token and SSH key are separate from this project and remain on your own devices.

## 3. Authorize your Spotify account

The library save button uses Spotify library permissions. When upgrading an existing installation to version 24, first stop the Spotify display, then run the auth command below once to grant those new permissions. Your existing token is replaced only after Spotify approval.

In the SSH window connected to the RM100, make the program executable and start sign-in. For an existing install, stop the app first so the normal interface returns:

    systemctl stop rm100-spotify.service

Then run:

    chmod +x /tmp/rm100_spotify
    /tmp/rm100_spotify auth

Keep that SSH window open. Open a **second PowerShell window on your computer** and create the local callback tunnel, using the same tablet IP:

    ssh -N -L 8080:127.0.0.1:8080 root@192.168.1.XX

Copy the full Spotify sign-in link printed in the first SSH window into your computer's browser. Sign in and approve access. When the first window says the sign-in was saved on the RM100, stop the tunnel in the second window with Ctrl+C. After authorization, keep Spotify stopped and continue to section 4, which installs the new version and starts it.

The refresh token is stored on the tablet at `/home/root/.rm100-spotify-token.json` with owner-only permissions. Do not copy it into this project or publish it.

## 4. Install automatic startup

In the RM100 SSH window, install the executable and service, then enable Spotify at boot:

    cp /tmp/rm100_spotify /home/root/rm100_spotify
    chmod +x /home/root/rm100_spotify
    cp /tmp/rm100-spotify.service /etc/systemd/system/rm100-spotify.service
    cp /tmp/rm100-spotify-start.timer /etc/systemd/system/rm100-spotify-start.timer
    systemctl daemon-reload
    systemctl disable xochitl.service
    systemctl disable rm100-spotify.service
    systemctl enable --now rm100-spotify-start.timer
    systemctl start rm100-spotify.service

The Spotify display should appear. At startup, the app takes over the screen from `xochitl`, the normal reMarkable interface. The EXIT SPOTIFY button returns to `xochitl`; a boot timer starts Spotify about 20 seconds after each reboot, once the reMarkable sync service has started. To start it again without rebooting, connect over SSH and run:

    systemctl start rm100-spotify.service

## 5. Controls

- Tap **PREVIOUS**, **PLAY / PAUSE**, or **NEXT** to control playback.
- Tap the large − and + symbols to adjust volume by 5% per tap.
- Tap **QUEUE** for the current track and upcoming queue. Tap an upcoming track to skip forward through Spotify's queue to that track, keeping the remaining queue or playlist going. Tap outside the queue panel or tap QUEUE again to close it.
- Tap **SHUFFLE** or **REPEAT OFF / ALL / ONE** to change playback mode.
- Tap **SAVE TRACK** to add the current song to your library; the button shows **SAVED** afterward.
- Tap **WIFI** in the upper-left header to see the connected network and IP address. Tap outside the panel to close it.
- Tap the small refresh icon to refresh playback, battery, and any open panels.
- Tap the device selector below PLAY / PAUSE in portrait, or between the volume controls in landscape, to choose a Spotify Connect device. The list opens downward and closes after you choose a device.
- Tap **ROTATE** to switch between portrait and landscape.
- Tap **EXIT SPOTIFY** to return to the normal reMarkable interface.
- The battery percentage refreshes every 10 seconds. A bolt appears beneath it while the tablet reports that its battery is charging.
- At 5% battery or lower, a full-screen low-battery alert appears and stays until the tablet reports that it is charging.

Playback controls require an active Spotify playback device. Spotify may reject volume changes if the account is not Premium or the active device does not support volume.

## Recovery and removal

If Spotify does not launch correctly, connect to the tablet over SSH and restore the normal reMarkable interface:

    systemctl stop rm100-spotify.service
    systemctl disable rm100-spotify.service
    systemctl disable rm100-spotify-start.timer
    systemctl enable xochitl.service
    systemctl start xochitl.service

To inspect the Spotify service log:

    journalctl -u rm100-spotify.service -n 50 --no-pager

For a USB recovery connection, reMarkable tablets commonly use `10.11.99.1`; connect the tablet to the computer with a data-capable USB cable and try `ssh root@10.11.99.1`.


















