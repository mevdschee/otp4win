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

// currentPalette builds the palette for the theme setting, following the
// Windows app mode for ThemeSystem and the system colors in high contrast.
func currentPalette(theme string) *palette {
	dark := theme == ThemeDark || theme == ThemeSystem && systemUsesDarkTheme()
	p := buildPalette(dark, highContrastOn(), sysColors{
		window:        fromColorref(win.GetSysColor(co.COLOR_WINDOW)),
		windowText:    fromColorref(win.GetSysColor(co.COLOR_WINDOWTEXT)),
		grayText:      fromColorref(win.GetSysColor(co.COLOR_GRAYTEXT)),
		btnFace:       fromColorref(win.GetSysColor(co.COLOR_BTNFACE)),
		highlight:     fromColorref(win.GetSysColor(co.COLOR_HIGHLIGHT)),
		highlightText: fromColorref(win.GetSysColor(co.COLOR_HIGHLIGHTTEXT)),
	})
	return &p
}

func fromColorref(c win.COLORREF) rgb {
	return rgb(c.Red())<<16 | rgb(c.Green())<<8 | rgb(c.Blue())
}

// cref converts to a GDI color.
func cref(c rgb) win.COLORREF { return win.RGB(c.r(), c.g(), c.b()) }

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

// highContrastOn reports whether a Windows high contrast theme is active.
func highContrastOn() bool {
	type highContrast struct {
		cbSize, dwFlags uint32
		scheme          uintptr
	}
	hc := highContrast{cbSize: uint32(unsafe.Sizeof(highContrast{}))}
	ret, _, _ := procSPI.Call(0x0042, uintptr(hc.cbSize), uintptr(unsafe.Pointer(&hc)), 0) // SPI_GETHIGHCONTRAST
	return ret != 0 && hc.dwFlags&1 != 0                                                   // HCF_HIGHCONTRASTON
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
	procSPI        = user32.NewProc("SystemParametersInfoW")
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

// setTheme sets the visual style of a control; name "" with classic set
// turns visual styles off so WM_CTLCOLOR* text colors apply.
func setTheme(h win.HWND, name string, classic bool) {
	if procSetTheme.Find() != nil {
		return
	}
	var p1, p2 uintptr
	if classic {
		empty := wstr.EncodeToSlice("")
		p1, p2 = uintptr(unsafe.Pointer(&empty[0])), uintptr(unsafe.Pointer(&empty[0]))
	} else if name != "" {
		p1 = uintptr(unsafe.Pointer(&wstr.EncodeToSlice(name)[0]))
	}
	procSetTheme.Call(uintptr(h), p1, p2)
}

// applyWindowTheme switches the title bar and all child controls of hwnd to
// dark or light. In dark mode, check boxes and group boxes lose their visual
// style, since that ignores the text color. Without dark mode support (old
// Windows versions) this silently does nothing.
func applyWindowTheme(hwnd win.HWND, p *palette) {
	hwnd.DwmSetWindowAttribute(win.DwmAttrUseImmersiveDarkMode(p.dark && !p.highContrast))
	dark := p.dark && !p.highContrast
	for _, h := range hwnd.EnumChildWindows() {
		cls, _ := h.GetClassName()
		style, _ := h.GetWindowLongPtr(co.GWLP_STYLE)
		switch {
		case !dark:
			setTheme(h, "", false) // back to the default visual style
		case cls == "Button" && isCheckOrGroup(co.BS(style&0x0f)):
			setTheme(h, "", true)
		case cls == "ComboBox" || cls == "Edit":
			setTheme(h, "DarkMode_CFD", false)
		default:
			setTheme(h, "DarkMode_Explorer", false)
		}
		h.InvalidateRect(nil, true)
	}
	hwnd.SetWindowPos(0, win.POINT{}, win.SIZE{},
		co.SWP_NOMOVE|co.SWP_NOSIZE|co.SWP_NOZORDER|co.SWP_FRAMECHANGED)
	hwnd.InvalidateRect(nil, true)
}

func isCheckOrGroup(bs co.BS) bool {
	switch bs {
	case co.BS_CHECKBOX, co.BS_AUTOCHECKBOX, co.BS_3STATE, co.BS_AUTO3STATE,
		co.BS_RADIOBUTTON, co.BS_AUTORADIOBUTTON, co.BS_GROUPBOX:
		return true
	}
	return false
}

// themeDialog makes a dialog follow the app palette: background, label and
// edit colors through WM_CTLCOLOR*, dark title bar and dark controls. The
// returned function must be called from the dialog's WM_CREATE handler and
// after a theme change.
func themeDialog(wnd *ui.Modal, a *app) (apply func()) {
	wnd.On().WmEraseBkgnd(func(p ui.WmEraseBkgnd) int {
		rc, _ := wnd.Hwnd().GetClientRect()
		p.Hdc().FillRect(&rc, a.bgBrush)
		return 1
	})
	static := func(p ui.WmCtlColor) win.HBRUSH {
		p.Hdc().SetTextColor(cref(a.pal.text))
		p.Hdc().SetBkColor(cref(a.pal.background))
		return a.bgBrush
	}
	edit := func(p ui.WmCtlColor) win.HBRUSH {
		p.Hdc().SetTextColor(cref(a.pal.editText))
		p.Hdc().SetBkColor(cref(a.pal.edit))
		return a.editBrush
	}
	wnd.On().WmCtlColorStatic(static)
	wnd.On().WmCtlColorBtn(static)
	wnd.On().WmCtlColorEdit(edit)
	wnd.On().WmCtlColorListBox(edit)
	return func() { applyWindowTheme(wnd.Hwnd(), a.pal) }
}

var boldUiFont win.HFONT

// sectionFont is the Windows message font in bold, for section titles.
func sectionFont() win.HFONT {
	if boldUiFont == 0 {
		var ncm win.NONCLIENTMETRICS
		ncm.SetCbSize()
		if win.SystemParametersInfo(co.SPI_GETNONCLIENTMETRICS, ncm.CbSize(), unsafe.Pointer(&ncm), 0) == nil {
			lf := ncm.LfMessageFont
			lf.Weight = co.FW_BOLD
			boldUiFont, _ = win.CreateFontIndirect(&lf)
		}
	}
	return boldUiFont
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
