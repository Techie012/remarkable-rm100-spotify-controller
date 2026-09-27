package main

import (
	"image"
	"os"
	"strconv"
	"strings"
)

const (
	paper = uint8(246)
	ink   = uint8(25)
	mid   = uint8(105)
	rule  = uint8(176)
)

// Compact built-in 5x7 lettering keeps the device binary self-contained.
var glyph = map[rune][7]string{
	'A':  {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B':  {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C':  {"01111", "10000", "10000", "10000", "10000", "10000", "01111"},
	'D':  {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E':  {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F':  {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G':  {"01111", "10000", "10000", "10111", "10001", "10001", "01111"},
	'H':  {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'I':  {"11111", "00100", "00100", "00100", "00100", "00100", "11111"},
	'J':  {"00111", "00010", "00010", "00010", "10010", "10010", "01100"},
	'K':  {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L':  {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M':  {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N':  {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'O':  {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'P':  {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q':  {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R':  {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S':  {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T':  {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U':  {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V':  {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W':  {"10001", "10001", "10001", "10101", "10101", "10101", "01010"},
	'X':  {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y':  {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z':  {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
	'0':  {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1':  {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2':  {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3':  {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4':  {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5':  {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6':  {"01110", "10000", "10000", "11110", "10001", "10001", "01110"},
	'7':  {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8':  {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9':  {"01110", "10001", "10001", "01111", "00001", "00001", "01110"},
	':':  {"00000", "00100", "00100", "00000", "00100", "00100", "00000"},
	'\'': {"00100", "00100", "01000", "00000", "00000", "00000", "00000"},
	'.':  {"00000", "00000", "00000", "00000", "00000", "00110", "00110"},
	',':  {"00000", "00000", "00000", "00000", "00110", "00100", "01000"},
	'!':  {"00100", "00100", "00100", "00100", "00100", "00000", "00100"},
	'%':  {"11001", "11010", "00100", "01000", "10110", "00110", "00000"},
	'+':  {"00000", "00100", "00100", "11111", "00100", "00100", "00000"},
	'?':  {"01110", "10001", "00001", "00010", "00100", "00000", "00100"},
	'&':  {"01100", "10010", "10100", "01000", "10101", "10010", "01101"},
	'<':  {"00010", "00100", "01000", "10000", "01000", "00100", "00010"},
	'(':  {"00010", "00100", "01000", "01000", "01000", "00100", "00010"},
	')':  {"01000", "00100", "00010", "00010", "00010", "00100", "01000"},
	'-':  {"00000", "00000", "00000", "11111", "00000", "00000", "00000"},
	'/':  {"00001", "00010", "00010", "00100", "01000", "01000", "10000"},
	' ':  {"00000", "00000", "00000", "00000", "00000", "00000", "00000"},
}

func (f *framebuffer) drawStatic(p *playback, cover *image.Image) error {
	w, h := f.size()
	f.fill(rect{0, 0, uint32(w), uint32(h)}, paper)
	if f.landscape {
		f.textFitCenter("NOW PLAYING", 76, 3, ink, w-400)
	} else {
		f.textFitCenter("NOW PLAYING", 84, 3, ink, w-360)
	}
	f.drawHomeButton()
	f.drawOrientationButton()
	f.drawNetworkButton()
	f.drawRefreshButton()
	f.drawBattery()

	if p == nil || p.Item == nil {
		f.drawEmpty()
		if f.lowBattery {
			f.drawLowBatteryScreen()
		}
		return f.refresh(rect{0, 0, uint32(w), uint32(h)}, waveGC16, updateFull)
	}

	coverBox := rect{left: 302, top: 190, width: 800, height: 800}
	if f.landscape {
		coverBox = rect{left: 88, top: 190, width: 720, height: 720}
	}
	f.fill(coverBox, 224)
	if cover != nil {
		f.drawCover(*cover, coverBox)
	} else {
		f.drawCoverPlaceholder(coverBox)
	}
	f.outline(coverBox, ink)

	artists := make([]string, 0, len(p.Item.Artists))
	for _, a := range p.Item.Artists {
		artists = append(artists, a.Name)
	}
	if f.landscape {
		textCenter := 1358
		f.textCenterAt(strings.ToUpper(p.Item.Name), textCenter, 490, 5, ink, 840)
		f.textCenterAt(strings.ToUpper(strings.Join(artists, " / ")), textCenter, 545, 3, mid, 840)
		f.textCenterAt(strings.ToUpper(p.Item.Album.Name), textCenter, 582, 2, mid, 840)
	} else {
		f.textFitCenter(strings.ToUpper(p.Item.Name), 1045, 6, ink, 1260)
		f.textFitCenter(strings.ToUpper(strings.Join(artists, " / ")), 1110, 4, mid, 1220)
		f.textFitCenter(strings.ToUpper(p.Item.Album.Name), 1165, 3, mid, 1220)
	}
	f.drawControls(p)
	deviceName := ""
	if p.Device != nil {
		deviceName = p.Device.Name
	}
	f.drawDeviceSelector(deviceName)
	f.drawFooter(w, h)
	if f.lowBattery {
		f.drawLowBatteryScreen()
	}
	return f.refresh(rect{0, 0, uint32(w), uint32(h)}, waveGC16, updateFull)
}

func (f *framebuffer) drawEmpty() {
	w, h := f.size()
	cx, cy := w/2, h/2-130
	if f.landscape {
		cx, cy = 448, 520
	} else {
		cy = 485
	}
	f.circle(cx, cy, 124, rule, false)
	f.circle(cx, cy, 119, mid, false)
	f.drawPlayIconAt(cx, cy, false)
	f.textCenterAt("READY FOR MUSIC", cx, cy+155, 6, ink, w-100)
	f.textCenterAt("START PLAYBACK FROM SPOTIFY", cx, cy+220, 3, mid, min(780, w-100))
	f.drawControls(nil)
	f.drawDeviceSelector("")
	f.drawFooter(w, h)
}

func (f *framebuffer) drawFooter(w, h int) {
	label := "SPOTIFY  /  RM100"
	color := mid
	if f.notice != "" {
		label, color = strings.ToUpper(f.notice), ink
	}
	y := 1810
	if f.landscape {
		y = h - 62
	}
	f.textFitCenter(label, y, 2, color, 900)
}

func deviceSelectorRect(landscape bool) rect {
	if landscape {
		return rect{top: 980, left: 235, width: 440, height: 70}
	}
	return rect{top: 1536, left: 452, width: 500, height: 72}
}

func deviceMenuRowCount(deviceCount int) int {
	if deviceCount <= 0 {
		return 1
	}
	if deviceCount > 4 {
		return 5
	}
	return deviceCount
}

func deviceMenuRect(landscape bool, deviceCount int) rect {
	button := deviceSelectorRect(landscape)
	rowHeight := uint32(48)
	if landscape {
		rowHeight = 55
	}
	return rect{top: button.top + button.height, left: button.left, width: button.width, height: rowHeight * uint32(deviceMenuRowCount(deviceCount))}
}

func (f *framebuffer) drawDeviceSelector(name string) {
	r := deviceSelectorRect(f.landscape)
	f.fill(r, paper)
	f.outline(r, ink)
	label := strings.TrimSpace(name)
	if label == "" {
		label = "SELECT DEVICE"
	}
	f.textCenterAt(strings.ToUpper(label), int(r.left+r.width/2), int(r.top)+(int(r.height)-14)/2, 2, ink, int(r.width)-24)
}

func (f *framebuffer) drawDeviceMenu(devices []spotifyDevice, offset int) {
	r := deviceMenuRect(f.landscape, len(devices))
	f.fill(r, paper)
	f.outline(r, ink)
	rowHeight := int(r.height) / deviceMenuRowCount(len(devices))
	if len(devices) == 0 {
		f.textCenterAt("NO DEVICES FOUND", int(r.left+r.width/2), int(r.top)+(int(r.height)-14)/2, 2, mid, int(r.width)-20)
		return
	}
	const pageSize = 4
	for row := 0; row < deviceMenuRowCount(len(devices)); row++ {
		label := ""
		index := offset + row
		switch {
		case row < pageSize && index < len(devices):
			label = devices[index].Name
			if devices[index].IsActive {
				label = "ACTIVE: " + label
			}
		case row == pageSize && len(devices) > pageSize:
			if offset+pageSize < len(devices) {
				label = "MORE DEVICES"
			} else {
				label = "FIRST PAGE"
			}
		}
		if label == "" {
			continue
		}
		y := int(r.top) + row*rowHeight
		if row > 0 {
			f.line(int(r.left)+8, y, int(r.left+r.width)-8, y, rule)
		}
		scale := 2
		if row == pageSize {
			scale = 2
		}
		textY := y + (rowHeight-7*scale)/2
		f.textCenterAt(strings.ToUpper(label), int(r.left+r.width/2), textY, scale, ink, int(r.width)-20)
	}
}

func (f *framebuffer) homeButton() rect {
	return rect{left: 60, top: 42, width: 232, height: 78}
}

func (f *framebuffer) drawHomeButton() {
	r := f.homeButton()
	f.outline(r, mid)
	f.textCenterAt("EXIT SPOTIFY", int(r.left+r.width/2), int(r.top)+(int(r.height)-14)/2, 2, ink, int(r.width)-20)
}
func (f *framebuffer) orientationButton() rect {
	w, _ := f.size()
	return rect{left: uint32(w - 370), top: 42, width: 232, height: 78}
}

func (f *framebuffer) drawBattery() {
	w, _ := f.size()
	capacity, err := os.ReadFile("/sys/class/power_supply/bq27441-0/capacity")
	if err != nil {
		f.fill(rect{top: 38, left: uint32(w - 132), width: 132, height: 70}, paper)
		f.textRight("--%", w-24, 68, 2, ink)
		f.batteryPercent = -1
		f.batteryCharging = false
		f.lowBattery = false
		return
	}
	percent, err := strconv.Atoi(strings.TrimSpace(string(capacity)))
	if err != nil {
		f.fill(rect{top: 38, left: uint32(w - 132), width: 132, height: 70}, paper)
		f.textRight("--%", w-24, 68, 2, ink)
		f.batteryPercent = -1
		f.batteryCharging = false
		f.lowBattery = false
		return
	}
	iconX, iconY := w-118, 51
	f.fill(rect{top: 38, left: uint32(w - 132), width: 132, height: 70}, paper)
	f.outline(rect{top: uint32(iconY), left: uint32(iconX), width: 34, height: 22}, ink)
	f.fill(rect{top: uint32(iconY + 6), left: uint32(iconX + 34), width: 4, height: 10}, ink)
	inner := max(0, min(26, percent*26/100))
	if inner > 0 {
		f.fill(rect{top: uint32(iconY + 4), left: uint32(iconX + 4), width: uint32(inner), height: 14}, ink)
	}
	f.text(strconv.Itoa(percent)+"%", iconX+44, iconY+3, 2, ink)
	status, _ := os.ReadFile("/sys/class/power_supply/bq27441-0/status")
	f.batteryCharging = strings.EqualFold(strings.TrimSpace(string(status)), "charging")
	// An empty marker enables a visual demo without modifying the battery gauge.
	if _, err := os.Stat("/tmp/rm100-force-low-battery-preview"); err == nil {
		percent = 5
		f.batteryCharging = false
	}
	if f.batteryCharging {
		f.drawChargingBolt(iconX+17, iconY+28)
	}
	f.batteryPercent = percent
	f.lowBattery = percent <= 5 && !f.batteryCharging
}

func (f *framebuffer) drawChargingBolt(cx, top int) {
	shape := [...]string{
		"00011000",
		"00011000",
		"00110000",
		"11111100",
		"00011000",
		"00110000",
		"00110000",
		"01100000",
		"01000000",
	}
	for y, row := range shape {
		for x, bit := range row {
			if bit == '1' {
				f.fill(rect{top: uint32(top + y*2), left: uint32(cx - 8 + x*2), width: 2, height: 2}, ink)
			}
		}
	}
}

func (f *framebuffer) refreshBattery() (bool, error) {
	previous := f.batteryPercent
	wasCharging := f.batteryCharging
	wasLow := f.lowBattery
	f.drawBattery()
	changed := previous != f.batteryPercent || wasCharging != f.batteryCharging || wasLow != f.lowBattery
	return changed, nil
}

func (f *framebuffer) drawLowBatteryScreen() {
	w, h := f.size()
	f.fill(rect{0, 0, uint32(w), uint32(h)}, paper)
	f.outline(rect{top: uint32(h/2 - 250), left: 100, width: uint32(w - 200), height: 500}, ink)
	f.textCenterAt("LOW BATTERY", w/2, h/2-170, 8, ink, w-300)
	f.textCenterAt("BATTERY < 5%", w/2, h/2-30, 5, ink, w-300)
	f.textCenterAt("PLEASE CONNECT USB-C", w/2, h/2+80, 4, ink, w-240)
	f.textCenterAt("THIS SCREEN WILL STAY UNTIL CHARGING", w/2, h/2+165, 2, mid, w-220)
}

func (f *framebuffer) drawOrientationButton() {
	r := f.orientationButton()
	f.outline(r, mid)
	f.textCenterAt("ROTATE", int(r.left+r.width/2), int(r.top)+(int(r.height)-14)/2, 2, ink, int(r.width)-24)
}

func networkButtonRect() rect { return rect{left: 320, top: 42, width: 110, height: 78} }
func refreshButtonRect() rect { return rect{left: 446, top: 42, width: 58, height: 78} }

func (f *framebuffer) drawNetworkButton() {
	r := networkButtonRect()
	f.outline(r, mid)
	f.textCenterAt("WIFI", int(r.left+r.width/2), int(r.top)+(int(r.height)-14)/2, 2, ink, int(r.width)-12)
}

func (f *framebuffer) drawRefreshButton() {
	r := refreshButtonRect()
	f.outline(r, mid)
	cx, cy := int(r.left+r.width/2), int(r.top+r.height/2)
	f.circle(cx, cy, 17, ink, false)
	// Leave a small break in the ring and add an arrow head to suggest refresh.
	f.fill(rect{top: uint32(cy - 20), left: uint32(cx - 4), width: 12, height: 8}, paper)
	for dy := 0; dy < 12; dy++ {
		for dx := 0; dx <= dy; dx++ {
			f.pixel(cx+5+dx, cy-20+dy, ink)
		}
	}
}

func (f *framebuffer) drawCoverPlaceholder(r rect) {
	x, y, w, h := int(r.left), int(r.top), int(r.width), int(r.height)
	f.fill(rect{uint32(y), uint32(x), uint32(w), uint32(h)}, 222)
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			n := 224 - int(80*float64(xx+yy)/float64(w+h))
			f.pixel(x+xx, y+yy, uint8(n))
		}
	}
	f.circle(x+w/2, y+h/2, 160, 242, true)
	f.circle(x+w/2, y+h/2, 160, ink, false)
	f.circle(x+w/2, y+h/2, 104, mid, false)
	f.textCenterAt("NO COVER", x+w/2, y+h/2-24, 5, ink, w-40)
	f.textCenterAt("ARTWORK", x+w/2, y+h/2+20, 4, mid, w-40)
}

func (f *framebuffer) drawCover(img image.Image, r rect) {
	x0, y0, size := int(r.left), int(r.top), int(r.width)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	crop := w
	if h < crop {
		crop = h
	}
	sx0, sy0 := b.Min.X+(w-crop)/2, b.Min.Y+(h-crop)/2
	for y := 0; y < size; y++ {
		sy := sy0 + y*crop/size
		for x := 0; x < size; x++ {
			sx := sx0 + x*crop/size
			rr, gg, bb, _ := img.At(sx, sy).RGBA()
			g := uint8((299*rr + 587*gg + 114*bb) / 1000 >> 8)
			// Four gray levels retain the album image's broad shapes on e-paper.
			level := (int(g) + 32) / 64
			if level >= 4 {
				g = 255
			} else {
				g = uint8(level * 64)
			}
			f.pixel(x0+x, y0+y, g)
		}
	}
}

func (f *framebuffer) drawControls(p *playback) {
	y, center := 1450, fbWidth/2
	playing := p != nil && p.IsPlaying
	volumeY := 1275
	if f.landscape {
		y, center, volumeY = 1030, 1360, 1030
	}
	previous, next := center-205, center+205
	volumeDown, volumeUp := previous, next
	if f.landscape {
		previous, next = center-220, center+220
		volumeDown, volumeUp = 130, 780
	}
	f.drawVolumeLabel(volumeDown, volumeY, "-")
	f.drawVolumeLabel(volumeUp, volumeY, "+")
	f.drawSkip(previous, y, false)
	f.drawPlayIconAt(center, y, playing)
	f.drawSkip(next, y, true)
	f.drawActionButtons(p)
}

func featureButtonRect(landscape bool, index int) rect {
	if landscape {
		return rect{top: 842, left: uint32(934 + index*218), width: 208, height: 58}
	}
	return rect{top: 1312, left: uint32(304 + index*204), width: 192, height: 58}
}

func featureButtonAt(p touchPoint, landscape bool) int {
	for index := 0; index < 4; index++ {
		if pointInRect(p, featureButtonRect(landscape, index)) {
			return index
		}
	}
	return -1
}

func (f *framebuffer) drawActionButtons(p *playback) {
	labels := [4]string{"QUEUE", "SHUFFLE", "REPEAT", "SAVE TRACK"}
	active := [4]bool{}
	enabled := [4]bool{true, true, true, p != nil && p.Item != nil}
	if p != nil {
		active[1] = p.Shuffle
		active[2] = p.Repeat != "off" && p.Repeat != ""
		active[3] = f.trackSaved
	}
	if p != nil && (p.Repeat == "context" || p.Repeat == "track") {
		if p.Repeat == "track" {
			labels[2] = "REPEAT ONE"
		} else {
			labels[2] = "REPEAT ALL"
		}
	} else {
		labels[2] = "REPEAT OFF"
	}
	if f.trackSaved {
		labels[3] = "SAVED"
	}
	for i, label := range labels {
		r := featureButtonRect(f.landscape, i)
		border, fg, bg := mid, ink, paper
		if active[i] {
			border, fg, bg = ink, paper, ink
		}
		if !enabled[i] {
			border, fg = rule, rule
		}
		f.fill(r, bg)
		f.outline(r, border)
		f.textCenterAt(label, int(r.left+r.width/2), int(r.top)+(int(r.height)-14)/2, 2, fg, int(r.width)-12)
	}
}

func queuePanelRect(landscape bool) rect {
	if landscape {
		return rect{top: 150, left: 850, width: 970, height: 1180}
	}
	return rect{top: 360, left: 180, width: 1044, height: 1120}
}

func queueCloseRect(landscape bool) rect {
	r := queuePanelRect(landscape)
	return rect{top: r.top + 24, left: r.left + r.width - 174, width: 146, height: 58}
}

func (f *framebuffer) drawQueueOverlay(q *playbackQueue) {
	r := queuePanelRect(f.landscape)
	f.fill(r, paper)
	f.outline(r, ink)
	f.textCenterAt("PLAYBACK QUEUE", int(r.left+r.width/2), int(r.top)+28, 5, ink, int(r.width)-210)
	close := queueCloseRect(f.landscape)
	f.outline(close, mid)
	f.textCenterAt("CLOSE", int(close.left+close.width/2), int(close.top)+20, 2, ink, int(close.width)-12)
	f.textCenterAt("TAP AN UPCOMING TRACK TO PLAY IT", int(r.left+r.width/2), int(r.top)+82, 2, mid, int(r.width)-60)
	f.line(int(r.left)+20, int(r.top)+102, int(r.left+r.width)-20, int(r.top)+102, rule)
	items := visibleQueueItems(q)
	if len(items) == 0 {
		f.textCenterAt("NOTHING IN THE QUEUE", int(r.left+r.width/2), int(r.top)+int(r.height)/2-10, 4, mid, int(r.width)-60)
		return
	}
	startY := int(r.top) + 116
	rowHeight := (int(r.height) - 138) / len(items)
	if rowHeight > 104 {
		rowHeight = 104
	}
	for i, item := range items {
		y := startY + i*rowHeight
		if i > 0 {
			f.line(int(r.left)+24, y, int(r.left+r.width)-24, y, rule)
		}
		title := item.Name
		if title == "" {
			title = "UNKNOWN ITEM"
		}
		f.textCenterAt(strings.ToUpper(title), int(r.left+r.width/2), y+12, 3, ink, int(r.width)-72)
		artists := make([]string, 0, len(item.Artists))
		for _, artist := range item.Artists {
			artists = append(artists, artist.Name)
		}
		detail := strings.Join(artists, " / ")
		if item.Type == "NOW PLAYING" {
			detail = "NOW PLAYING"
		} else if detail == "" {
			detail = strings.ToUpper(item.Type)
		}
		if detail != "" {
			f.textCenterAt(strings.ToUpper(detail), int(r.left+r.width/2), y+48, 2, mid, int(r.width)-72)
		}
	}
}

func networkPanelRect(landscape bool) rect {
	if landscape {
		return rect{top: 370, left: 850, width: 970, height: 610}
	}
	return rect{top: 560, left: 180, width: 1044, height: 640}
}

func (f *framebuffer) drawNetworkOverlay(info networkInfo) {
	r := networkPanelRect(f.landscape)
	f.fill(r, paper)
	f.outline(r, ink)
	f.textCenterAt("WIFI AND NETWORK", int(r.left+r.width/2), int(r.top)+42, 5, ink, int(r.width)-60)
	state, ssid, address := "DISCONNECTED", "UNKNOWN WIFI NETWORK", "NO IP ADDRESS"
	if info.Connected {
		state, ssid = "CONNECTED", "CONNECTED WIFI"
	}
	if info.SSID != "" {
		ssid = strings.ToUpper(info.SSID)
	}
	if info.IPv4 != "" {
		address = info.IPv4
	}
	f.textCenterAt(state, int(r.left+r.width/2), int(r.top)+155, 4, mid, int(r.width)-80)
	f.textCenterAt("NETWORK", int(r.left+r.width/2), int(r.top)+245, 2, mid, int(r.width)-80)
	f.textCenterAt(ssid, int(r.left+r.width/2), int(r.top)+280, 4, ink, int(r.width)-80)
	f.textCenterAt("IP ADDRESS", int(r.left+r.width/2), int(r.top)+390, 2, mid, int(r.width)-80)
	f.textCenterAt(address, int(r.left+r.width/2), int(r.top)+425, 4, ink, int(r.width)-80)
	f.textCenterAt("TAP ANYWHERE TO CLOSE", int(r.left+r.width/2), int(r.top)+int(r.height)-52, 2, mid, int(r.width)-80)
}

func (f *framebuffer) drawVolumeLabel(cx, cy int, label string) {
	// Large, slightly emboldened labels replace the directional volume arrows.
	const scale = 8
	x := cx - len(label)*6*scale/2
	y := cy - 7*scale/2
	f.text(label, x, y, scale, ink)
	f.text(label, x+1, y, scale, ink)
}

func (f *framebuffer) drawPlayIcon(playing bool) {
	cx, cy := fbWidth/2, 1450
	if f.landscape {
		cx, cy = 1360, 1030
	}
	f.drawPlayIconAt(cx, cy, playing)
}

func (f *framebuffer) drawPlayIconAt(cx, cy int, playing bool) {
	// Clear the previous symbol before drawing the new play/pause state.
	f.circle(cx, cy, 56, paper, true)
	f.circle(cx, cy, 56, ink, false)
	f.circle(cx, cy, 54, ink, false)
	if playing {
		f.fill(rect{uint32(cy - 23), uint32(cx - 17), 13, 46}, ink)
		f.fill(rect{uint32(cy - 23), uint32(cx + 5), 13, 46}, ink)
	} else {
		for dy := -32; dy <= 32; dy++ {
			width := max(1, 54*(32-abs(dy))/32)
			for dx := 0; dx < width; dx++ {
				f.pixel(cx-26+dx, cy+dy, ink)
			}
		}
	}
}

func (f *framebuffer) drawSkip(cx, cy int, next bool) {
	if next {
		f.fill(rect{top: uint32(cy - 29), left: uint32(cx + 22), width: 7, height: 58}, ink)
		for dy := -28; dy <= 28; dy++ {
			inset := abs(dy) * 34 / 28
			for x := cx - 23; x <= cx+16-inset; x++ {
				f.pixel(x, cy+dy, ink)
			}
		}
	} else {
		f.fill(rect{top: uint32(cy - 29), left: uint32(cx - 29), width: 7, height: 58}, ink)
		for dy := -28; dy <= 28; dy++ {
			inset := abs(dy) * 34 / 28
			for x := cx - 16 + inset; x <= cx+23; x++ {
				f.pixel(x, cy+dy, ink)
			}
		}
	}
}

func (f *framebuffer) textCenter(s string, y, scale int, color uint8, maxWidth int) {
	f.textFitCenter(s, y, scale, color, maxWidth)
}

func (f *framebuffer) textFitCenter(s string, y, scale int, color uint8, maxWidth int) {
	s = normalize(s)
	if s == "" {
		return
	}
	if len(s)*(6*scale) > maxWidth {
		scale = max(1, maxWidth/(len(s)*6))
	}
	width := len(s) * 6 * scale
	f.textAt(s, (fbWidth-width)/2, y, scale, color)
}

func (f *framebuffer) textCenterAt(s string, cx, y, scale int, color uint8, maxWidth int) {
	s = normalize(s)
	if len(s)*(6*scale) > maxWidth {
		scale = max(1, maxWidth/(len(s)*6))
	}
	f.textAt(s, cx-len(s)*6*scale/2, y, scale, color)
}

func (f *framebuffer) text(s string, x, y, scale int, color uint8) {
	f.textAt(normalize(s), x, y, scale, color)
}

func (f *framebuffer) textRight(s string, right, y, scale int, color uint8) {
	s = normalize(s)
	f.textAt(s, right-len(s)*6*scale, y, scale, color)
}

func normalize(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return strings.Map(func(r rune) rune {
		if _, ok := glyph[r]; ok {
			return r
		}
		return '?'
	}, s)
}

func (f *framebuffer) textAt(s string, x, y, scale int, color uint8) {
	if scale < 1 {
		scale = 1
	}
	for _, ch := range s {
		g, ok := glyph[ch]
		if !ok {
			g = glyph['?']
		}
		for gy, row := range g {
			for gx, on := range row {
				if on != '1' {
					continue
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						f.pixel(x+gx*scale+dx, y+gy*scale+dy, color)
					}
				}
			}
		}
		x += 6 * scale
	}
}

func (f *framebuffer) line(x1, y1, x2, y2 int, color uint8) {
	dx, dy := abs(x2-x1), -abs(y2-y1)
	sx, sy := -1, -1
	if x1 < x2 {
		sx = 1
	}
	if y1 < y2 {
		sy = 1
	}
	err := dx + dy
	for {
		f.pixel(x1, y1, color)
		if x1 == x2 && y1 == y2 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x1 += sx
		}
		if e2 <= dx {
			err += dx
			y1 += sy
		}
	}
}

func (f *framebuffer) outline(r rect, color uint8) {
	x, y, w, h := int(r.left), int(r.top), int(r.width), int(r.height)
	f.line(x, y, x+w-1, y, color)
	f.line(x, y+h-1, x+w-1, y+h-1, color)
	f.line(x, y, x, y+h-1, color)
	f.line(x+w-1, y, x+w-1, y+h-1, color)
}

func (f *framebuffer) circle(cx, cy, radius int, color uint8, fill bool) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			d := x*x + y*y
			if (fill && d <= radius*radius) || (!fill && d <= radius*radius && d >= (radius-2)*(radius-2)) {
				f.pixel(cx+x, cy+y, color)
			}
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
