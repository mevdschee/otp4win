package main

import (
	"unsafe"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/wstr"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// palette holds the colors used for the custom painted parts of the UI. The
// values are open-otp's Material light and dark color schemes.
type palette struct {
	dark           bool
	background     win.COLORREF
	onBackground   win.COLORREF
	card           win.COLORREF // surface tinted at elevation, like ElevatedCard
	cardSelected   win.COLORREF
	onSurface      win.COLORREF
	surfaceVariant win.COLORREF
	onSurfaceVar   win.COLORREF
	outline        win.COLORREF
	primary        win.COLORREF
	errorColor     win.COLORREF
}

var lightPalette = palette{
	background:     win.RGB(0xFE, 0xFC, 0xF5),
	onBackground:   win.RGB(0x1B, 0x1C, 0x18),
	card:           win.RGB(0xF3, 0xF4, 0xE7),
	cardSelected:   win.RGB(0xDD, 0xE6, 0xC6), // secondaryContainer
	onSurface:      win.RGB(0x1B, 0x1C, 0x18),
	surfaceVariant: win.RGB(0xE1, 0xE4, 0xD4),
	onSurfaceVar:   win.RGB(0x45, 0x48, 0x3D),
	outline:        win.RGB(0x75, 0x78, 0x6C),
	primary:        win.RGB(0x46, 0x68, 0x00),
	errorColor:     win.RGB(0xBA, 0x1A, 0x1A),
}

var darkPalette = palette{
	dark:           true,
	background:     win.RGB(0x1B, 0x1C, 0x18),
	onBackground:   win.RGB(0xE4, 0xE3, 0xDB),
	card:           win.RGB(0x25, 0x28, 0x1F),
	cardSelected:   win.RGB(0x41, 0x4A, 0x32), // secondaryContainer
	onSurface:      win.RGB(0xE4, 0xE3, 0xDB),
	surfaceVariant: win.RGB(0x45, 0x48, 0x3D),
	onSurfaceVar:   win.RGB(0xC5, 0xC8, 0xB9),
	outline:        win.RGB(0x8F, 0x92, 0x85),
	primary:        win.RGB(0xAB, 0xD4, 0x68),
	errorColor:     win.RGB(0xFF, 0xB4, 0xAB),
}

func paletteFor(theme string) *palette {
	switch theme {
	case ThemeDark:
		return &darkPalette
	case ThemeLight:
		return &lightPalette
	}
	if systemUsesDarkTheme() {
		return &darkPalette
	}
	return &lightPalette
}

// systemUsesDarkTheme reads the "Choose your app mode" setting.
func systemUsesDarkTheme() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && v == 0
}

// lerpColor mixes a and b, t in [0,1].
func lerpColor(a, b win.COLORREF, t float64) win.COLORREF {
	t = max(0, min(1, t))
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return win.RGB(mix(a.Red(), b.Red()), mix(a.Green(), b.Green()), mix(a.Blue(), b.Blue()))
}

func solidBrush(c win.COLORREF) win.HBRUSH {
	h, _ := win.CreateBrushIndirect(&win.LOGBRUSH{Style: co.BRS_SOLID, Color: c})
	return h
}

var (
	user32         = windows.NewLazySystemDLL("user32.dll")
	uxtheme        = windows.NewLazySystemDLL("uxtheme.dll")
	procAppendMenu = user32.NewProc("AppendMenuW")
	procSetTheme   = uxtheme.NewProc("SetWindowTheme")
)

const (
	mfString    = 0x0000
	mfGrayed    = 0x0001
	mfSeparator = 0x0800
)

func appendMenu(h win.HMENU, flags uintptr, id uint16, text string) {
	var p *uint16
	if text != "" {
		p = &wstr.EncodeToSlice(text)[0]
	}
	procAppendMenu.Call(uintptr(h), flags, uintptr(id), uintptr(unsafe.Pointer(p)))
}

// applyWindowTheme switches the title bar and the common controls of hwnd
// (and the given children) to dark or light. Unsupported on old Windows
// versions, where it silently does nothing.
func applyWindowTheme(hwnd win.HWND, p *palette, children ...win.HWND) {
	hwnd.DwmSetWindowAttribute(win.DwmAttrUseImmersiveDarkMode(p.dark))
	name := "Explorer"
	if p.dark {
		name = "DarkMode_Explorer"
	}
	for _, h := range children {
		if procSetTheme.Find() == nil {
			procSetTheme.Call(uintptr(h), uintptr(unsafe.Pointer(&wstr.EncodeToSlice(name)[0])), 0)
		}
		h.InvalidateRect(nil, true)
	}
	hwnd.SetWindowPos(0, win.POINT{}, win.SIZE{},
		co.SWP_NOMOVE|co.SWP_NOSIZE|co.SWP_NOZORDER|co.SWP_FRAMECHANGED)
}

// makeFont creates a font of the given point size scaled to the system DPI.
func makeFont(face string, points int, bold bool) win.HFONT {
	var lf win.LOGFONT
	lf.Height = -int32(ui.DpiY(points * 96 / 72))
	lf.Weight = co.FW_NORMAL
	if bold {
		lf.Weight = co.FW_BOLD
	}
	lf.CharSet = co.CHARSET_DEFAULT
	lf.Quality = co.QUALITY_DEFAULT // let RDP pick, it often disables ClearType
	lf.SetFaceName(face)
	h, _ := win.CreateFontIndirect(&lf)
	return h
}
