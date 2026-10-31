//go:build windows

// Command otp is a TOTP client for Windows with the features of
// open-otp (https://github.com/avan1235/open-otp). It uses plain Win32
// controls and GDI drawing (no OpenGL), so it stays responsive over RDP.
package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/wstr"
)

const (
	iconId       = 1 // RT_GROUP_ICON "#1" in winres/winres.json
	timerTick    = 1
	timerToast   = 2
	tickInterval = 250 // ms; the list only repaints when something changed
)

type app struct {
	wnd         *ui.Main
	txtSearch   *ui.Edit
	btnSettings *ui.Button
	list        *codeList
	lblStatus   *ui.Static
	btnAdd      *ui.Button

	accounts []Account
	prefs    Preferences
	rows     []Row
	query    string

	pal       *palette
	bgBrush   win.HBRUSH
	editBrush win.HBRUSH
}

func main() {
	runtime.LockOSThread() // Windows GUI is single-threaded

	const w, h = 380, 540
	wnd := ui.NewMain(
		ui.OptsMain().
			Title("otp4win").
			Size(ui.Dpi(w, h)).
			Center(true).
			ClassIconId(iconId).
			ClassBrush(win.HBRUSH(0)).
			AccelTable(accelerators()).
			Style(co.WS_OVERLAPPEDWINDOW | co.WS_CLIPCHILDREN | co.WS_VISIBLE),
	)
	me := &app{wnd: wnd}
	// Standard control sizes: edits 23 px and buttons 88x26 px high (at
	// 96 DPI), with 12 px margins and 8 px spacing.
	const margin, gap, btnW, btnH, editH = 12, 8, 88, 26, 23
	me.txtSearch = ui.NewEdit(wnd,
		ui.OptsEdit().
			Position(ui.Dpi(margin, margin+(btnH-editH)/2)).
			Width(ui.DpiX(w-2*margin-gap-btnW)).
			Layout(ui.LAY_RESIZE_HOLD),
	)
	me.btnSettings = ui.NewButton(wnd,
		ui.OptsButton().
			Text("&Settings").
			Position(ui.Dpi(w-margin-btnW, margin)).
			Layout(ui.LAY_MOVE_HOLD),
	)
	top := margin + btnH + gap
	bottom := margin + btnH + gap
	x, y := ui.Dpi(0, top)
	cx, cy := ui.Dpi(w, h-top-bottom)
	me.list = newCodeList(me, wnd, x, y, cx, cy)
	me.lblStatus = ui.NewStatic(wnd,
		ui.OptsStatic().
			Position(ui.Dpi(margin, h-margin-btnH+5)).
			Size(ui.Dpi(w-2*margin-gap-btnW, 16)).
			CtrlStyle(co.SS_ENDELLIPSIS|co.SS_NOPREFIX).
			Layout(ui.LAY_RESIZE_MOVE),
	)
	me.btnAdd = ui.NewButton(wnd,
		ui.OptsButton().
			Text("&Add...").
			Position(ui.Dpi(w-margin-btnW, h-margin-btnH)).
			Layout(ui.LAY_MOVE_MOVE),
	)
	me.events()
	wnd.RunAsMain()
}

func (me *app) events() {
	me.wnd.On().WmCreate(func(_ ui.WmCreate) int {
		me.prefs = loadPreferences()
		me.applyTheme()
		me.txtSearch.Hwnd().SendMessage(co.EM_SETCUEBANNER, 1,
			win.LPARAM(unsafe.Pointer(&wstr.EncodeToSlice("Search...")[0])))
		accounts, err := loadAccounts()
		if err != nil {
			me.showError("Could not load saved codes", err)
		}
		me.accounts = accounts
		me.refresh()
		me.wnd.Hwnd().SetTimer(timerTick, tickInterval)
		me.checkForUpdate()
		me.list.hwnd().SetFocus()
		return 0
	})

	me.wnd.On().WmDestroy(func() {
		for _, b := range []win.HBRUSH{me.bgBrush, me.editBrush} {
			if b != 0 {
				b.DeleteObject()
			}
		}
	})

	me.wnd.On().WmGetMinMaxInfo(func(p ui.WmGetMinMaxInfo) {
		p.Info().PtMinTrackSize = win.POINT{X: int32(ui.DpiX(300)), Y: int32(ui.DpiY(260))}
	})

	me.wnd.On().WmEraseBkgnd(func(p ui.WmEraseBkgnd) int {
		rc, _ := me.wnd.Hwnd().GetClientRect()
		p.Hdc().FillRect(&rc, me.bgBrush)
		return 1
	})
	me.wnd.On().WmCtlColorStatic(func(p ui.WmCtlColor) win.HBRUSH {
		p.Hdc().SetTextColor(cref(me.pal.text))
		p.Hdc().SetBkColor(cref(me.pal.background))
		return me.bgBrush
	})
	me.wnd.On().WmCtlColorBtn(func(p ui.WmCtlColor) win.HBRUSH {
		return me.bgBrush
	})
	me.wnd.On().WmCtlColorEdit(func(p ui.WmCtlColor) win.HBRUSH {
		p.Hdc().SetTextColor(cref(me.pal.editText))
		p.Hdc().SetBkColor(cref(me.pal.edit))
		return me.editBrush
	})
	// Follow changes of the Windows app mode, high contrast and colors.
	for _, msg := range []co.WM{co.WM(0x001a), co.WM_SYSCOLORCHANGE, co.WM_THEMECHANGED} { // 0x1a: WM_SETTINGCHANGE
		me.wnd.On().Wm(msg, func(p ui.Wm) uintptr {
			if *currentPalette(me.prefs.Theme) != *me.pal {
				me.applyTheme()
			}
			return 0
		})
	}

	me.wnd.On().WmTimer(timerTick, func() { me.list.tick(time.Now()) })
	me.wnd.On().WmTimer(timerToast, func() {
		me.wnd.Hwnd().KillTimer(timerToast)
		me.lblStatus.Hwnd().SetWindowText("")
	})

	me.txtSearch.On().EnChange(func() {
		me.query = me.txtSearch.Text()
		me.list.scroll = 0
		me.refresh()
	})

	// Esc clears the search, like open-otp's back arrow.
	me.wnd.On().WmCommand(uint16(co.ID_CANCEL), co.CMD_MENU, func() {
		if me.txtSearch.Text() != "" {
			me.txtSearch.SetText("")
		}
		me.list.hwnd().SetFocus()
	})

	me.btnSettings.On().BnClicked(func() { showSettings(me) })
	me.btnAdd.On().BnClicked(func() { showAddDialog(me, nil) })

	// Keyboard shortcuts that work anywhere in the main window.
	me.wnd.On().WmCommandAccelMenu(cmdFind, func() {
		me.txtSearch.Focus()
		me.txtSearch.SetSelection(0, -1)
	})
	me.wnd.On().WmCommandAccelMenu(cmdAdd, func() { showAddDialog(me, nil) })
	me.wnd.On().WmCommandAccelMenu(cmdPaste, func() {
		if me.pasteURI() {
			return
		}
		if win.GetFocus() == me.txtSearch.Hwnd() {
			me.txtSearch.Hwnd().SendMessage(co.WM_PASTE, 0, 0)
		}
	})
}

const (
	cmdFind = 200 + iota
	cmdAdd
	cmdPaste
)

func accelerators() win.HACCEL {
	h, _ := win.CreateAcceleratorTable(
		win.ACCEL{FVirt: co.ACCELF_VIRTKEY | co.ACCELF_CONTROL, Key: co.VK('F'), Cmd: cmdFind},
		win.ACCEL{FVirt: co.ACCELF_VIRTKEY | co.ACCELF_CONTROL, Key: co.VK('N'), Cmd: cmdAdd},
		win.ACCEL{FVirt: co.ACCELF_VIRTKEY | co.ACCELF_CONTROL, Key: co.VK('V'), Cmd: cmdPaste},
		win.ACCEL{FVirt: co.ACCELF_VIRTKEY | co.ACCELF_SHIFT, Key: co.VK_INSERT, Cmd: cmdPaste},
	)
	return h
}

// refresh recomputes the visible rows and repaints the list, keeping the
// selection on the same account.
func (me *app) refresh() {
	sel := me.list.selectedAccount()
	me.rows = presentRows(me.accounts, me.prefs, me.query)
	me.list.selected = -1
	if sel >= 0 {
		me.list.selectAccount(sel)
	}
	if me.list.selected < 0 && len(me.rows) > 0 && me.query != "" {
		me.list.selected = me.list.nextAccountRow(-1, 1)
	}
	me.list.updateScroll()
}

func (me *app) save() {
	if err := saveAccounts(me.accounts); err != nil {
		me.showError("Could not save codes", err)
	}
}

func (me *app) savePrefs() {
	if err := savePreferences(me.prefs); err != nil {
		me.showError("Could not save settings", err)
	}
}

// canReorder reports whether manual reordering is possible: only when the
// list is not sorted (as in open-otp) and not filtered by a search.
func (me *app) canReorder() bool {
	return me.prefs.SortBy == SortNone && me.query == ""
}

func (me *app) copyCode(idx int) {
	if idx < 0 || idx >= len(me.accounts) {
		return
	}
	code, err := me.accounts[idx].Code(time.Now())
	if err != nil {
		me.showError("Could not generate code", err)
		return
	}
	if err := me.copyToClipboard(code); err != nil {
		me.showError("Could not copy to clipboard", err)
		return
	}
	me.toast(fmt.Sprintf("Copied \"%s\" to clipboard", code))
}

func (me *app) deleteAccount(idx int) {
	if idx < 0 || idx >= len(me.accounts) {
		return
	}
	if me.prefs.ConfirmDelete {
		prompt := "Are you sure you want to delete this item?"
		if name := me.accounts[idx].NamePresentation(); name != "" {
			prompt = "Are you sure you want to delete\n" + name + "?"
		}
		if ret, _ := me.wnd.Hwnd().MessageBox(prompt, "Delete", co.MB_YESNO|co.MB_ICONQUESTION|co.MB_DEFBUTTON2); ret != co.ID_YES {
			return
		}
	}
	row := me.list.selected
	me.accounts = append(me.accounts[:idx], me.accounts[idx+1:]...)
	me.save()
	me.rows = presentRows(me.accounts, me.prefs, me.query)
	me.list.selected = -1
	if n := me.list.nextAccountRow(row-1, 1); n >= 0 {
		me.list.selected = n
	} else {
		me.list.selected = me.list.nextAccountRow(len(me.rows), -1)
	}
	me.list.updateScroll()
}

func (me *app) moveAccount(from, to int) {
	if !me.canReorder() || from == to {
		me.list.invalidate()
		return
	}
	me.accounts = moveAccount(me.accounts, from, to)
	me.save()
	me.rows = presentRows(me.accounts, me.prefs, me.query)
	me.list.selectAccount(to)
	me.list.updateScroll()
}

func (me *app) moveSelected(dir int) {
	idx := me.list.selectedAccount()
	if idx < 0 || !me.canReorder() {
		return
	}
	to := idx + dir
	if to < 0 || to >= len(me.accounts) {
		return
	}
	me.moveAccount(idx, to)
}

// addAccounts appends new accounts and selects the last one.
func (me *app) addAccounts(accounts ...Account) {
	for i := range accounts {
		accounts[i].ID = newID()
	}
	me.accounts = append(me.accounts, accounts...)
	me.save()
	me.txtSearch.SetText("") // fires refresh
	me.refresh()
	me.list.selectAccount(len(me.accounts) - 1)
	me.list.hwnd().SetFocus()
}

// pasteURI adds the accounts of an otpauth:// or otpauth-migration:// URI on
// the clipboard, like fyne-authenticator's paste shortcut. It returns false
// when the clipboard holds something else.
func (me *app) pasteURI() bool {
	text := me.clipboardText()
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(text)), "otpauth") {
		return false
	}
	accounts, err := parseOtpURIs(text)
	if err != nil {
		me.showError("Could not read the pasted URI", err)
		return true
	}
	for _, a := range accounts {
		if !validSecret(a.Secret) {
			// Open the form so the missing or broken secret can be fixed.
			showAddDialog(me, &a)
			return true
		}
	}
	me.addAccounts(accounts...)
	if len(accounts) == 1 {
		me.toast("Added " + orDefault(accounts[0].NamePresentation(), "code"))
	} else {
		me.toast(fmt.Sprintf("Added %d codes", len(accounts)))
	}
	return true
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func (me *app) toast(msg string) {
	me.lblStatus.Hwnd().SetWindowText(msg)
	me.wnd.Hwnd().SetTimer(timerToast, 2500)
}

func (me *app) applyTheme() {
	me.pal = currentPalette(me.prefs.Theme)
	for _, b := range []win.HBRUSH{me.bgBrush, me.editBrush} {
		if b != 0 {
			b.DeleteObject()
		}
	}
	me.bgBrush = solidBrush(cref(me.pal.background))
	me.editBrush = solidBrush(cref(me.pal.edit))
	applyWindowTheme(me.wnd.Hwnd(), me.pal)
	me.list.invalidate()
}

func (me *app) copyToClipboard(text string) error {
	hClip, err := win.OpenClipboard(me.wnd.Hwnd())
	if err != nil {
		return err
	}
	defer hClip.CloseClipboard()
	if err := hClip.EmptyClipboard(); err != nil {
		return err
	}
	utf16 := wstr.EncodeToSlice(text) // null-terminated
	data := unsafe.Slice((*byte)(unsafe.Pointer(&utf16[0])), len(utf16)*2)
	return hClip.SetClipboardData(co.CF_UNICODETEXT, data)
}

func (me *app) clipboardText() string {
	hClip, err := win.OpenClipboard(me.wnd.Hwnd())
	if err != nil {
		return ""
	}
	defer hClip.CloseClipboard()
	if ok, _ := hClip.IsClipboardFormatAvailable(co.CF_UNICODETEXT); !ok {
		return ""
	}
	data, err := hClip.GetClipboardData(co.CF_UNICODETEXT)
	if err != nil || len(data) < 2 {
		return ""
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(&data[0])), len(data)/2)
	for i, c := range u {
		if c == 0 {
			u = u[:i]
			break
		}
	}
	return string(utf16.Decode(u))
}

func (me *app) showError(msg string, err error) {
	me.wnd.Hwnd().MessageBox(msg+":\n"+err.Error(), "otp4win", co.MB_ICONERROR)
}
