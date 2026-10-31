package main

import (
	"math"
	"strings"
	"time"
	"unicode"
	"unsafe"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

// codeList is a custom painted, scrollable list of code cards, laid out like
// open-otp's main screen: issuer icon, "Issuer: account" line, the code and
// a countdown bar. It is drawn with plain
// GDI into a back buffer and only repainted when something visible changes.
type codeList struct {
	ctl *ui.Control
	app *app

	scroll   int // pixels scrolled from the top
	selected int // row index or -1

	press    int // row pressed with the left button, or -1
	pressPos win.POINT
	dragging bool
	dragY    int // mouse y while dragging, in client coordinates
	dragOff  int // mouse y minus card top at the start of the drag

	fontName, fontCode, fontIcon, fontHeader, fontEmpty win.HFONT

	shown string // signature of the last painted state, see tick
}

// Metrics in pixels at 96 DPI, scaled by m().
const (
	mMargin  = 12
	mCardH   = 72
	mGap     = 12
	mHeaderH = 30
	mIcon    = 44
	mBarW    = 6
	mBarH    = 46
	mTrail   = 34 // width of the area holding the countdown bar
)

func m(v int) int { return ui.DpiY(v) }

func newCodeList(a *app, parent ui.Parent, x, y, cx, cy int) *codeList {
	me := &codeList{app: a, selected: -1, press: -1}
	me.ctl = ui.NewControl(parent,
		ui.OptsControl().
			Position(x, y).
			Size(cx, cy).
			Layout(ui.LAY_RESIZE_RESIZE).
			ClassBrush(win.HBRUSH(0)).
			ExStyle(co.WS_EX_LEFT).
			Style(co.WS_CHILD|co.WS_VISIBLE|co.WS_TABSTOP|co.WS_VSCROLL|co.WS_CLIPSIBLINGS),
	)
	me.events()
	return me
}

func (me *codeList) hwnd() win.HWND { return me.ctl.Hwnd() }

func (me *codeList) events() {
	on := me.ctl.On()
	on.WmCreate(func(_ ui.WmCreate) int {
		me.fontName = makeFont("Segoe UI", 9, false)
		me.fontCode = makeFont("Consolas", 19, true)
		me.fontIcon = makeFont("Segoe UI", 14, true)
		me.fontHeader = makeFont("Segoe UI", 10, true)
		me.fontEmpty = makeFont("Segoe UI", 11, false)
		return 0
	})
	on.WmDestroy(func() {
		for _, f := range []win.HFONT{me.fontName, me.fontCode, me.fontIcon, me.fontHeader, me.fontEmpty} {
			if f != 0 {
				f.DeleteObject()
			}
		}
	})
	on.WmEraseBkgnd(func(_ ui.WmEraseBkgnd) int { return 1 })
	on.WmPaint(me.paint)
	on.WmSize(func(_ ui.WmSize) { me.updateScroll() })
	on.WmVScroll(me.onVScroll)
	on.Wm(co.WM(0x020a), func(p ui.Wm) uintptr { // WM_MOUSEWHEEL
		delta := int(int16(p.WParam.HiWord()))
		me.scrollTo(me.scroll - delta*m(mCardH+mGap)/120)
		return 0
	})
	on.WmSetFocus(func(_ ui.WmSetFocus) {
		if me.selected < 0 {
			me.selectRow(me.nextAccountRow(-1, 1))
		}
		me.invalidate()
	})
	on.WmKillFocus(func(_ ui.WmKillFocus) { me.invalidate() })
	on.WmGetDlgCode(func(p ui.WmGetDlgCode) co.DLGC {
		code := co.DLGC_WANTARROWS | co.DLGC_WANTCHARS
		if msg := p.Message(); msg != nil && msg.Msg == co.WM_KEYDOWN && co.VK(msg.WParam) == co.VK_RETURN {
			code |= co.DLGC_WANTMESSAGE
		}
		return code
	})
	on.WmKeyDown(me.onKeyDown)
	on.WmLButtonDown(me.onLButtonDown)
	on.WmLButtonDblClk(me.onLButtonDown)
	on.WmMouseMove(me.onMouseMove)
	on.WmLButtonUp(me.onLButtonUp)
	on.WmCaptureChanged(func(_ ui.WmCaptureChanged) {
		if me.dragging {
			me.dragging = false
			me.invalidate()
		}
		me.press = -1
	})
	on.WmContextMenu(me.onContextMenu)
}

// mousePos decodes signed client coordinates from a mouse message.
func mousePos(p ui.WmMouse) win.POINT {
	return win.POINT{X: int32(int16(p.Raw.LParam.LoWord())), Y: int32(int16(p.Raw.LParam.HiWord()))}
}

func (me *codeList) invalidate() {
	me.shown = ""
	me.hwnd().InvalidateRect(nil, false)
}

// --- layout ---

func (me *codeList) rowHeight(r Row) int {
	if r.IsHeader {
		return m(mHeaderH)
	}
	return m(mCardH + mGap)
}

func (me *codeList) rowTop(i int) int {
	y := m(mGap)
	for n := 0; n < i && n < len(me.app.rows); n++ {
		y += me.rowHeight(me.app.rows[n])
	}
	return y
}

func (me *codeList) contentHeight() int {
	return me.rowTop(len(me.app.rows))
}

// rowAt returns the row under client y, or -1.
func (me *codeList) rowAt(y int) int {
	top := m(mGap) - me.scroll
	for i, r := range me.app.rows {
		h := me.rowHeight(r)
		if y >= top && y < top+h {
			if r.IsHeader || y < top+m(mCardH) {
				return i
			}
			return -1 // in the gap below a card
		}
		top += h
	}
	return -1
}

func (me *codeList) clientSize() (int, int) {
	rc, _ := me.hwnd().GetClientRect()
	return int(rc.Right), int(rc.Bottom)
}

func (me *codeList) cardRect(top int) win.RECT {
	w, _ := me.clientSize()
	return win.RECT{Left: int32(m(mMargin)), Top: int32(top), Right: int32(w - m(mMargin)), Bottom: int32(top + m(mCardH))}
}

// trailingRect is the area at the right of a card that holds the countdown.
func (me *codeList) trailingRect(card win.RECT) win.RECT {
	s := int32(m(mTrail))
	cy := (card.Top + card.Bottom) / 2
	r := card.Right - int32(m(14))
	return win.RECT{Left: r - s, Top: cy - s/2, Right: r, Bottom: cy + s/2}
}

func (me *codeList) updateScroll() {
	_, h := me.clientSize()
	si := win.SCROLLINFO{Mask: co.SIF_RANGE | co.SIF_PAGE | co.SIF_POS}
	si.SetCbSize()
	si.Min = 0
	si.Max = int32(me.contentHeight())
	si.Page = uint32(h + 1)
	me.scroll = max(0, min(me.scroll, me.contentHeight()-h))
	si.Pos = int32(me.scroll)
	me.hwnd().SetScrollInfo(co.SBB_VERT, &si, true)
	me.invalidate()
}

func (me *codeList) scrollTo(y int) {
	_, h := me.clientSize()
	y = max(0, min(y, me.contentHeight()-h))
	if y == me.scroll {
		return
	}
	me.scroll = y
	me.updateScroll()
}

func (me *codeList) onVScroll(p ui.WmScroll) {
	_, h := me.clientSize()
	line := m(mCardH + mGap)
	switch p.Request() {
	case co.SB_REQ_LINEUP:
		me.scrollTo(me.scroll - line)
	case co.SB_REQ_LINEDOWN:
		me.scrollTo(me.scroll + line)
	case co.SB_REQ_PAGEUP:
		me.scrollTo(me.scroll - h)
	case co.SB_REQ_PAGEDOWN:
		me.scrollTo(me.scroll + h)
	case co.SB_REQ_TOP:
		me.scrollTo(0)
	case co.SB_REQ_BOTTOM:
		me.scrollTo(me.contentHeight())
	case co.SB_REQ_THUMBTRACK, co.SB_REQ_THUMBPOSITION:
		si := win.SCROLLINFO{Mask: co.SIF_TRACKPOS}
		si.SetCbSize()
		me.hwnd().GetScrollInfo(co.SBB_VERT, &si)
		me.scrollTo(int(si.TrackPos))
	}
}

// ensureVisible scrolls so that row i is fully shown.
func (me *codeList) ensureVisible(i int) {
	if i < 0 || i >= len(me.app.rows) {
		return
	}
	_, h := me.clientSize()
	top := me.rowTop(i)
	if i > 0 && me.app.rows[i-1].IsHeader {
		top = me.rowTop(i - 1)
	}
	bottom := me.rowTop(i) + m(mCardH+mGap)
	switch {
	case top-m(mGap) < me.scroll:
		me.scrollTo(top - m(mGap))
	case bottom > me.scroll+h:
		me.scrollTo(bottom - h)
	}
}

// --- selection and keyboard ---

func (me *codeList) nextAccountRow(from, dir int) int {
	for i := from + dir; i >= 0 && i < len(me.app.rows); i += dir {
		if !me.app.rows[i].IsHeader {
			return i
		}
	}
	return -1
}

func (me *codeList) selectRow(i int) {
	if i < 0 {
		return
	}
	me.selected = i
	me.ensureVisible(i)
	me.invalidate()
}

// selectedAccount returns the account index of the selected row, or -1.
func (me *codeList) selectedAccount() int {
	if me.selected < 0 || me.selected >= len(me.app.rows) || me.app.rows[me.selected].IsHeader {
		return -1
	}
	return me.app.rows[me.selected].Index
}

// selectAccount selects the row showing account index idx.
func (me *codeList) selectAccount(idx int) {
	for i, r := range me.app.rows {
		if !r.IsHeader && r.Index == idx {
			me.selectRow(i)
			return
		}
	}
	me.selected = -1
}

func (me *codeList) onKeyDown(p ui.WmKey) {
	ctrl, _ := win.GetKeyState(co.VK_CONTROL)
	switch p.VirtualKeyCode() {
	case co.VK_UP:
		if ctrl {
			me.app.moveSelected(-1)
		} else if n := me.nextAccountRow(me.selected, -1); n >= 0 {
			me.selectRow(n)
		}
	case co.VK_DOWN:
		if ctrl {
			me.app.moveSelected(1)
		} else if n := me.nextAccountRow(me.selected, 1); n >= 0 {
			me.selectRow(n)
		}
	case co.VK_HOME:
		me.selectRow(me.nextAccountRow(-1, 1))
	case co.VK_END:
		me.selectRow(me.nextAccountRow(len(me.app.rows), -1))
	case co.VK_RETURN, co.VK_SPACE:
		me.app.copyCode(me.selectedAccount())
	case co.VK_DELETE:
		me.app.deleteAccount(me.selectedAccount())
	case co.VK('C'):
		if ctrl {
			me.app.copyCode(me.selectedAccount())
		}
	}
}

// --- mouse ---

func (me *codeList) onLButtonDown(p ui.WmMouse) {
	me.hwnd().SetFocus()
	pt := mousePos(p)
	row := me.rowAt(int(pt.Y))
	me.press, me.pressPos = -1, pt
	if row < 0 || me.app.rows[row].IsHeader {
		return
	}
	me.selected = row
	me.press = row
	me.hwnd().SetCapture()
	me.invalidate()
}

func (me *codeList) onMouseMove(p ui.WmMouse) {
	pt := mousePos(p)
	if me.press < 0 {
		return
	}
	if !me.dragging && me.app.canReorder() &&
		abs(int(pt.Y-me.pressPos.Y)) > m(6) {
		me.dragging = true
		me.dragOff = int(me.pressPos.Y) - (me.rowTop(me.press) - me.scroll)
	}
	if me.dragging {
		me.dragY = int(pt.Y)
		_, h := me.clientSize()
		if pt.Y < 0 {
			me.scrollTo(me.scroll - m(mGap))
		} else if int(pt.Y) > h {
			me.scrollTo(me.scroll + m(mGap))
		}
		me.invalidate()
	}
}

func (me *codeList) onLButtonUp(p ui.WmMouse) {
	pt := mousePos(p)
	press, dragging := me.press, me.dragging
	me.press, me.dragging = -1, false
	win.ReleaseCapture()
	if press < 0 {
		return
	}
	if dragging {
		target := me.dropIndex(int(pt.Y))
		me.app.moveAccount(me.app.rows[press].Index, target)
		return
	}
	if me.rowAt(int(pt.Y)) != press {
		return
	}
	me.app.copyCode(me.app.rows[press].Index)
}

// dropIndex is the account position the dragged card lands on. Dragging is
// only enabled for the unsorted, unfiltered list, where rows map 1:1 to
// accounts.
func (me *codeList) dropIndex(y int) int {
	center := y - me.dragOff + m(mCardH)/2 + me.scroll
	i := (center - m(mGap)) / m(mCardH+mGap)
	return max(0, min(i, len(me.app.rows)-1))
}

const (
	cmdCopy = 100 + iota
	cmdMoveUp
	cmdMoveDown
	cmdDelete
)

func (me *codeList) onContextMenu(p ui.WmContextMenu) {
	pos := p.CursorPos()
	if pos.X == -1 && pos.Y == -1 || int16(pos.X) == -1 && int16(pos.Y) == -1 { // keyboard
		if me.selected < 0 {
			return
		}
		pt := win.POINT{X: int32(m(mMargin * 3)), Y: int32(me.rowTop(me.selected) - me.scroll + m(mCardH)/2)}
		me.hwnd().ClientToScreenPt(&pt)
		pos = pt
	} else {
		pos = win.POINT{X: int32(int16(pos.X)), Y: int32(int16(pos.Y))}
		pt := pos
		me.hwnd().ScreenToClientPt(&pt)
		row := me.rowAt(int(pt.Y))
		if row < 0 || me.app.rows[row].IsHeader {
			return
		}
		me.selected = row
		me.invalidate()
	}
	idx := me.selectedAccount()
	if idx < 0 {
		return
	}
	menu, err := win.CreatePopupMenu()
	if err != nil {
		return
	}
	defer menu.DestroyMenu()
	appendMenu(menu, mfString, cmdCopy, "&Copy\tEnter")
	appendMenu(menu, mfSeparator, 0, "")
	moveFlags := uintptr(mfString)
	if !me.app.canReorder() {
		moveFlags |= mfGrayed
	}
	appendMenu(menu, moveFlags, cmdMoveUp, "Move &up\tCtrl+Up")
	appendMenu(menu, moveFlags, cmdMoveDown, "Move &down\tCtrl+Down")
	appendMenu(menu, mfSeparator, 0, "")
	appendMenu(menu, mfString, cmdDelete, "&Delete\tDel")

	cmd, _ := menu.TrackPopupMenu(co.TPM_RETURNCMD|co.TPM_RIGHTBUTTON, int(pos.X), int(pos.Y), me.hwnd())
	switch cmd {
	case cmdCopy:
		me.app.copyCode(idx)
	case cmdMoveUp:
		me.app.moveSelected(-1)
	case cmdMoveDown:
		me.app.moveSelected(1)
	case cmdDelete:
		me.app.deleteAccount(idx)
	}
}

// --- painting ---

// tick is called by the main window timer. It repaints only when a visible
// code, countdown step or color changed, which keeps RDP traffic low.
func (me *codeList) tick(now time.Time) {
	var b strings.Builder
	_, h := me.clientSize()
	top := m(mGap) - me.scroll
	for _, r := range me.app.rows {
		rh := me.rowHeight(r)
		if top+rh > 0 && top < h && !r.IsHeader {
			a := &me.app.accounts[r.Index]
			code, _ := a.Code(now)
			b.WriteString(code)
			b.WriteByte(byte(countdownSteps(a, now)))
		}
		top += rh
	}
	if s := b.String(); s != me.shown {
		me.hwnd().InvalidateRect(nil, false)
	}
}

// countdownSteps quantizes the countdown to 2 steps per second.
func countdownSteps(a *Account, now time.Time) int {
	return int(math.Ceil(a.TimeLeft(now) * float64(a.period()) * 2))
}

func (me *codeList) paint() {
	var ps win.PAINTSTRUCT
	hdc, err := me.hwnd().BeginPaint(&ps)
	if err != nil {
		return
	}
	defer me.hwnd().EndPaint(&ps)

	w, h := me.clientSize()
	if w <= 0 || h <= 0 {
		return
	}
	// Back buffer as a 32-bit DIB section, so shapes can be drawn with
	// anti-aliasing in Go (canvas) and text with GDI on the same pixels.
	mem, _ := hdc.CreateCompatibleDC()
	defer mem.DeleteDC()
	var bmi win.BITMAPINFO
	bmi.BmiHeader.SetBiSize()
	bmi.BmiHeader.Width = int32(w)
	bmi.BmiHeader.Height = -int32(h) // top-down
	bmi.BmiHeader.Planes = 1
	bmi.BmiHeader.BitCount = 32
	bmi.BmiHeader.Compression = co.BI_RGB
	bmp, bits, err := mem.CreateDIBSection(&bmi, co.DIB_COLORS_RGB, 0, 0)
	if err != nil {
		return
	}
	defer bmp.DeleteObject()
	oldBmp, _ := mem.SelectObject(win.HGDIOBJ(bmp))
	defer mem.SelectObject(oldBmp)
	cv := &canvas{pix: unsafe.Slice((*uint32)(unsafe.Pointer(bits)), w*h), w: w, h: h}

	pal := me.app.pal
	cv.fill(pal.background)
	mem.SetBkMode(co.BKMODE_TRANSPARENT)

	now := time.Now()
	var sig strings.Builder

	if len(me.app.accounts) == 0 {
		mem.SelectObjectFont(me.fontEmpty)
		mem.SetTextColor(cref(pal.text))
		rc := win.RECT{Right: int32(w), Bottom: int32(h)}
		mem.DrawText("Add New Keys", &rc, co.DT_CENTER|co.DT_VCENTER|co.DT_SINGLELINE)
	}

	focused := win.GetFocus() == me.hwnd()
	top := m(mGap) - me.scroll
	for i, r := range me.app.rows {
		rh := me.rowHeight(r)
		visible := top+rh > 0 && top < h
		switch {
		case !visible:
		case r.IsHeader:
			mem.SelectObjectFont(me.fontHeader)
			mem.SetTextColor(cref(pal.text))
			rc := win.RECT{Left: int32(m(mMargin + 4)), Top: int32(top), Right: int32(w - m(mMargin)), Bottom: int32(top + rh - m(4))}
			mem.DrawText(r.Header, &rc, co.DT_LEFT|co.DT_BOTTOM|co.DT_SINGLELINE|co.DT_END_ELLIPSIS|co.DT_NOPREFIX)
		case me.dragging && i == me.press:
			// leave the slot empty, the card follows the mouse below
		default:
			me.paintCard(mem, cv, i, top, now, i == me.selected, focused, &sig)
		}
		top += rh
	}
	if me.dragging && me.press >= 0 && me.press < len(me.app.rows) {
		me.paintCard(mem, cv, me.press, me.dragY-me.dragOff, now, true, true, &sig)
	}
	me.shown = sig.String()

	win.GdiFlush()
	hdc.BitBlt(win.POINT{}, win.SIZE{Cx: int32(w), Cy: int32(h)}, mem, win.POINT{}, co.ROP_SRCCOPY)
}

// warnSeconds is when the countdown bar switches to the warning color.
const warnSeconds = 5

// paintCard draws one card: the shapes on the canvas (anti-aliased), then
// the text with GDI. GdiFlush keeps the two in order, since GDI may batch.
func (me *codeList) paintCard(hdc win.HDC, cv *canvas, row, top int, now time.Time, selected, focused bool, sig *strings.Builder) {
	c := &me.app.pal.card
	if selected {
		c = &me.app.pal.selected
	}
	a := &me.app.accounts[me.app.rows[row].Index]
	card := me.cardRect(top)
	f := func(v int32) float64 { return float64(v) }
	win.GdiFlush()

	// Card, with a thicker border when it has the keyboard focus.
	bw := float64(max(1, m(1)))
	if selected && focused {
		bw = float64(m(2))
	}
	cv.card(f(card.Left), f(card.Top), f(card.Right), f(card.Bottom), float64(m(6)), bw, c.fill, c.border)

	// Leading issuer "icon": a circle with the first letter.
	cy := f(card.Top+card.Bottom) / 2
	rIcon := float64(m(mIcon)) / 2
	cxIcon := f(card.Left) + float64(m(14)) + rIcon
	cv.fillCircle(cxIcon, cy, rIcon, c.iconFill)

	// Trailing countdown bar, in the warning color for the last seconds.
	trail := me.trailingRect(card)
	code, err := a.Code(now)
	if err != nil {
		code = strings.Repeat("-", a.digits())
	}
	sig.WriteString(code)
	steps := countdownSteps(a, now)
	sig.WriteByte(byte(steps))
	left := float64(steps) / (float64(a.period()) * 2)
	bar := c.bar
	if left*float64(a.period()) <= warnSeconds {
		bar = c.warn
	}
	barW, barH := float64(m(mBarW)), float64(m(mBarH))
	bx := f(trail.Left+trail.Right)/2 - barW/2
	by := cy - barH/2
	cv.fillRoundRect(bx, by, bx+barW, by+barH, barW/2, c.track)
	cv.fillRoundRect(bx, by+barH*(1-left), bx+barW, by+barH, barW/2, bar)

	// Text on top of the shapes.
	icon := win.RECT{
		Left: int32(cxIcon - rIcon), Top: int32(cy - rIcon),
		Right: int32(cxIcon + rIcon), Bottom: int32(cy + rIcon),
	}
	hdc.SelectObjectFont(me.fontIcon)
	hdc.SetTextColor(cref(c.iconText))
	hdc.DrawText(initial(a), &icon, co.DT_CENTER|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX)

	textLeft := icon.Right + int32(m(16))
	textRight := trail.Left - int32(m(8))
	name := a.NamePresentation()
	codeRc := win.RECT{Left: textLeft, Top: card.Top, Right: textRight, Bottom: card.Bottom}
	if name != "" {
		hdc.SelectObjectFont(me.fontName)
		hdc.SetTextColor(cref(c.subtext))
		nameRc := win.RECT{Left: textLeft, Top: card.Top + int32(m(10)), Right: textRight, Bottom: card.Top + int32(m(28))}
		hdc.DrawText(name, &nameRc, co.DT_LEFT|co.DT_SINGLELINE|co.DT_END_ELLIPSIS|co.DT_NOPREFIX)
		codeRc.Top = card.Top + int32(m(26))
		codeRc.Bottom = card.Bottom - int32(m(8))
	}
	hdc.SelectObjectFont(me.fontCode)
	hdc.SetTextColor(cref(c.text))
	hdc.DrawText(splitCode(code), &codeRc, co.DT_LEFT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX)
	win.GdiFlush()
}

// initial picks the letter shown in the issuer circle.
func initial(a *Account) string {
	for _, s := range []string{a.Issuer, a.AccountName} {
		for _, r := range strings.TrimSpace(s) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return string(unicode.ToUpper(r))
			}
		}
	}
	return "#"
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
