package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	evSyn = 0
	evKey = 1
	evAbs = 3

	btnTouch      = 330
	btnToolPen    = 320
	btnToolFinger = 325

	absX            = 0
	absY            = 1
	absMTPositionX  = 53
	absMTPositionY  = 54
	absMTTrackingID = 57
)

type absInfo struct {
	Value, Min, Max, Fuzz, Flat, Resolution int32
}

type touchPoint struct{ x, y int }

type touchDevice struct {
	file                   *os.File
	name                   string
	xInfo, yInfo           absInfo
	altXInfo, altYInfo     absInfo
	xCode, yCode           uint16
	altXCode, altYCode     uint16
	hasPrimary, hasAltAxes bool
	invertX, invertY       bool
}

func absRange(fd uintptr, code uint16) (absInfo, bool) {
	var info absInfo
	request := uintptr(0x80184540) + uintptr(code)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(unsafe.Pointer(&info)))
	return info, errno == 0 && info.Max > info.Min
}

func startTouchReader() <-chan touchPoint {
	points := make(chan touchPoint, 8)
	paths, _ := filepath.Glob("/dev/input/event*")
	var touchscreens, penDevices, fallback []*touchDevice
	for _, path := range paths {
		file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			continue
		}
		fd := file.Fd()
		mtX, mtXOK := absRange(fd, absMTPositionX)
		mtY, mtYOK := absRange(fd, absMTPositionY)
		absXInfo, absXOK := absRange(fd, absX)
		absYInfo, absYOK := absRange(fd, absY)
		if (!mtXOK || !mtYOK) && (!absXOK || !absYOK) {
			file.Close()
			continue
		}
		nameBytes, _ := os.ReadFile(strings.Replace(path, "/dev/input/", "/sys/class/input/", 1) + "/device/name")
		name := strings.TrimSpace(string(nameBytes))
		nameLower := strings.ToLower(name)
		isFingerTouchscreen := strings.Contains(nameLower, "cyttsp") || strings.Contains(nameLower, "touchscreen") || strings.Contains(nameLower, "touch screen")
		isPenDigitizer := strings.Contains(nameLower, "wacom") || strings.Contains(nameLower, "digitizer") || strings.Contains(nameLower, "pen")
		device := &touchDevice{file: file, name: name}
		if mtXOK && mtYOK {
			device.xInfo, device.yInfo = mtX, mtY
			device.xCode, device.yCode = absMTPositionX, absMTPositionY
			device.hasPrimary = true
			if absXOK && absYOK {
				device.altXInfo, device.altYInfo = absXInfo, absYInfo
				device.altXCode, device.altYCode = absX, absY
				device.hasAltAxes = true
			}
		} else {
			device.xInfo, device.yInfo = absXInfo, absYInfo
			device.xCode, device.yCode = absX, absY
			device.hasPrimary = true
		}
		if isFingerTouchscreen {
			device.invertX, device.invertY = true, true
			touchscreens = append(touchscreens, device)
		} else if isPenDigitizer {
			penDevices = append(penDevices, device)
		} else {
			fallback = append(fallback, device)
		}
	}
	devices := touchscreens
	if len(devices) == 0 {
		devices = fallback
	}
	if len(devices) == 0 {
		devices = penDevices
	}
	for _, group := range [][]*touchDevice{touchscreens, penDevices, fallback} {
		for _, candidate := range group {
			keep := false
			for _, selected := range devices {
				if candidate == selected {
					keep = true
					break
				}
			}
			if !keep {
				candidate.file.Close()
			}
		}
	}
	if len(devices) == 0 {
		return nil
	}
	for _, d := range devices {
		fmt.Printf("Touch: device %s, primary axes %d/%d range X %d..%d Y %d..%d", d.name, d.xCode, d.yCode, d.xInfo.Min, d.xInfo.Max, d.yInfo.Min, d.yInfo.Max)
		if d.hasAltAxes {
			fmt.Printf(", also listening to axes %d/%d", d.altXCode, d.altYCode)
		}
		fmt.Println()
		go d.read(points)
	}
	return points
}

func (d *touchDevice) read(out chan<- touchPoint) {
	defer d.file.Close()
	buf := make([]byte, 16)
	var rawX, rawY, altRawX, altRawY int32
	var primaryXSeen, primaryYSeen, altXSeen, altYSeen bool
	down := false
	beginContact := func(source string) {
		if !down {
			down = true
			primaryXSeen, primaryYSeen, altXSeen, altYSeen = false, false, false, false
			fmt.Printf("Touch: contact started (%s)\n", source)
		}
	}
	finishContact := func(source string) {
		if !down {
			return
		}
		down = false
		switch {
		case primaryXSeen && primaryYSeen:
			fmt.Printf("Touch: contact ended (%s), primary raw=%d,%d\n", source, rawX, rawY)
			sendTouch(out, rawX, rawY, d.xInfo, d.yInfo, d.invertX, d.invertY)
		case d.hasAltAxes && altXSeen && altYSeen:
			fmt.Printf("Touch: contact ended (%s), alternate raw=%d,%d\n", source, altRawX, altRawY)
			sendTouch(out, altRawX, altRawY, d.altXInfo, d.altYInfo, d.invertX, d.invertY)
		default:
			fmt.Printf("Touch: contact ended (%s), coordinates were not reported during contact\n", source)
		}
	}
	for {
		n, err := d.file.Read(buf)
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				time.Sleep(8 * time.Millisecond)
				continue
			}
			fmt.Printf("Touch: reader for %s stopped: %v\n", d.name, err)
			return
		}
		if n < 16 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		typ := uint16(buf[8]) | uint16(buf[9])<<8
		code := uint16(buf[10]) | uint16(buf[11])<<8
		value := int32(uint32(buf[12]) | uint32(buf[13])<<8 | uint32(buf[14])<<16 | uint32(buf[15])<<24)

		switch typ {
		case evAbs:
			switch code {
			case d.xCode:
				rawX, primaryXSeen = value, true
			case d.yCode:
				rawY, primaryYSeen = value, true
			case d.altXCode:
				if d.hasAltAxes {
					altRawX, altXSeen = value, true
				}
			case d.altYCode:
				if d.hasAltAxes {
					altRawY, altYSeen = value, true
				}
			case absMTTrackingID:
				if value >= 0 {
					beginContact("multitouch tracking id")
				} else {
					finishContact("multitouch tracking id")
				}
			}
		case evKey:
			if code == btnTouch || code == btnToolPen || code == btnToolFinger {
				if value != 0 {
					beginContact(fmt.Sprintf("key %d", code))
				} else {
					finishContact(fmt.Sprintf("key %d", code))
				}
			}
		case evSyn:
		}
	}
}

func sendTouch(out chan<- touchPoint, rawX, rawY int32, xi, yi absInfo, invertX, invertY bool) {
	x := scaleTouch(rawX, xi, fbWidth)
	y := scaleTouch(rawY, yi, fbHeight)
	if invertX {
		x = fbWidth - 1 - x
	}
	if invertY {
		y = fbHeight - 1 - y
	}
	select {
	case out <- touchPoint{x: x, y: y}:
	default:
	}
}

func scaleTouch(value int32, info absInfo, pixels int) int {
	v := int64(value-info.Min) * int64(pixels-1) / int64(info.Max-info.Min)
	if v < 0 {
		return 0
	}
	if v >= int64(pixels) {
		return pixels - 1
	}
	return int(v)
}

func (f *framebuffer) logicalTouch(p touchPoint) touchPoint {
	if f.landscape {
		return touchPoint{x: p.y, y: fbWidth - 1 - p.x}
	}
	return p
}
