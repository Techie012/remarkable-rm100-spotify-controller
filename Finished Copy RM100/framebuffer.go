package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	fbStrideBytes   = 2816
	fbMapBytes      = 2816 * 3840
	fbWidth         = 1404
	fbHeight        = 1872
	ioctlSendUpdate = 0x4048462e
	waveGC16        = 2
	updatePartial   = 0
	updateFull      = 1
)

type rect struct {
	top, left, width, height uint32
}

// This is the 72-byte mxcfb_update_data layout used by the RM1-family EPDC.
type altBuffer struct {
	PhysAddr uint32
	Width    uint32
	Height   uint32
	Region   rect
}

type updateData struct {
	Region      rect
	Waveform    uint32
	Mode        uint32
	Marker      uint32
	Temperature int32
	Flags       uint32
	DitherMode  uint32
	QuantBit    uint32
	Alt         altBuffer
}

type framebuffer struct {
	file            *os.File
	mem             []byte
	marker          uint32
	landscape       bool
	batteryPercent  int
	batteryCharging bool
	lowBattery      bool
	trackSaved      bool
	notice          string
}

func openFramebuffer() (*framebuffer, error) {
	f, err := os.OpenFile("/dev/fb0", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/fb0: %w", err)
	}
	mem, err := syscall.Mmap(int(f.Fd()), 0, fbMapBytes, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("map /dev/fb0: %w", err)
	}
	return &framebuffer{file: f, mem: mem, batteryPercent: -2}, nil
}

func (f *framebuffer) close() {
	_ = syscall.Munmap(f.mem)
	_ = f.file.Close()
}

func (f *framebuffer) pixel(x, y int, gray uint8) {
	w, h := f.size()
	if x < 0 || x >= w || y < 0 || y >= h {
		return
	}
	if f.landscape {
		x, y = fbWidth-1-y, x
	}
	v := uint16(gray>>3)<<11 | uint16(gray>>2)<<5 | uint16(gray>>3)
	i := y*fbStrideBytes + x*2
	f.mem[i] = byte(v)
	f.mem[i+1] = byte(v >> 8)
}

func (f *framebuffer) fill(r rect, gray uint8) {
	w, h := f.size()
	x1, y1 := int(r.left), int(r.top)
	x2, y2 := x1+int(r.width), y1+int(r.height)
	if x1 < 0 {
		x1 = 0
	}
	if y1 < 0 {
		y1 = 0
	}
	if x2 > w {
		x2 = w
	}
	if y2 > h {
		y2 = h
	}
	for y := y1; y < y2; y++ {
		for x := x1; x < x2; x++ {
			f.pixel(x, y, gray)
		}
	}
}

func (f *framebuffer) refresh(r rect, waveform, mode uint32) error {
	if f.landscape {
		r = rect{
			top:    r.left,
			left:   uint32(fbWidth) - (r.top + r.height),
			width:  r.height,
			height: r.width,
		}
	}
	f.marker++
	data := updateData{
		Region: r, Waveform: waveform, Mode: mode, Marker: f.marker,
		Temperature: 0x1001, Flags: 0, DitherMode: 0, QuantBit: 0,
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.file.Fd(), uintptr(ioctlSendUpdate), uintptr(unsafe.Pointer(&data)))
	if errno != 0 {
		return fmt.Errorf("EPDC refresh ioctl failed: %v", errno)
	}
	return nil
}

func (f *framebuffer) size() (int, int) {
	if f.landscape {
		return fbHeight, fbWidth
	}
	return fbWidth, fbHeight
}

func (f *framebuffer) controlRefreshRect() rect {
	if f.landscape {
		return rect{top: 960, left: 1090, width: 540, height: 135}
	}
	return rect{top: 1380, left: 460, width: 485, height: 140}
}
