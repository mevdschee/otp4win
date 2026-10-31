package main

import (
	"slices"
	"strings"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

// addDialog is open-otp's add screen with its "Account Details" and
// "Advanced Settings" groups (TOTP only).
type addDialog struct {
	app       *app
	wnd       *ui.Modal
	issuer    *ui.Edit
	name      *ui.Edit
	secret    *ui.Edit
	algorithm *ui.ComboBox
	digits    *ui.ComboBox
	period    *ui.ComboBox
	init      *Account
	busy      bool // set while filling the form from an URI
}

// showAddDialog opens the add screen, optionally pre-filled.
func showAddDialog(a *app, init *Account) {
	const w, h = 360, 366
	me := &addDialog{app: a, init: init}
	me.wnd = ui.NewModal(a.wnd,
		ui.OptsModal().
			Title("Add code").
			Size(ui.Dpi(w, h)).
			Style(co.WS_CAPTION|co.WS_SYSMENU|co.WS_BORDER|co.WS_VISIBLE),
	)
	const lx, ex, ew = 24, 130, w - 130 - 24
	// Section titles in bold, like the Windows 11 settings pages. Group
	// boxes are avoided since they ignore the text color in dark mode.
	var titles []*ui.Static
	group := func(text string, y, _ int) {
		titles = append(titles, ui.NewStatic(me.wnd, ui.OptsStatic().
			Text(text).
			Position(ui.Dpi(14, y)).Size(ui.Dpi(w-28, 20))))
	}
	label := func(text string, y int) {
		ui.NewStatic(me.wnd, ui.OptsStatic().Text(text).Position(ui.Dpi(lx, y+3)))
	}
	edit := func(y int, style co.ES) *ui.Edit {
		return ui.NewEdit(me.wnd, ui.OptsEdit().
			Position(ui.Dpi(ex, y)).Width(ui.DpiX(ew)).
			CtrlStyle(co.ES_AUTOHSCROLL|style))
	}
	combo := func(y int, items []string, sel int) *ui.ComboBox {
		return ui.NewComboBox(me.wnd, ui.OptsComboBox().
			Position(ui.Dpi(ex, y)).Width(ui.DpiX(ew)).
			CtrlStyle(co.CBS_DROPDOWNLIST).
			Texts(items...).Select(sel))
	}

	group("Account Details", 10, 160)
	label("&Issuer", 34)
	me.issuer = edit(32, 0)
	label("Account &Name", 66)
	me.name = edit(64, 0)
	label("S&ecret", 98)
	me.secret = edit(96, co.ES_PASSWORD)
	ui.NewStatic(me.wnd, ui.OptsStatic().
		Text("Paste an otpauth:// link in any field to fill in the form.").
		Position(ui.Dpi(lx, 130)).Size(ui.Dpi(w-2*lx, 32)))

	group("Advanced Settings", 180, 126)
	label("&Algorithm", 204)
	me.algorithm = combo(204, Algorithms, 0)
	label("&Digits", 236)
	me.digits = combo(236, []string{"6", "8"}, 0)
	label("&Period", 268)
	me.period = combo(268, []string{"15 seconds", "30 seconds", "60 seconds"}, 1)

	btnSave := ui.NewButton(me.wnd,
		ui.OptsButton().
			Text("&Save").
			Position(ui.Dpi(w-10-88-8-88, h-44)).
			Width(ui.DpiX(88)).
			CtrlStyle(co.BS_DEFPUSHBUTTON),
	)
	btnCancel := ui.NewButton(me.wnd,
		ui.OptsButton().
			Text("Cancel").
			Position(ui.Dpi(w-10-88, h-44)).
			Width(ui.DpiX(88)),
	)

	applyTheme := themeDialog(me.wnd, a)
	me.wnd.On().WmCreate(func(_ ui.WmCreate) int {
		applyTheme()
		for _, t := range titles {
			t.Hwnd().SendMessage(co.WM_SETFONT, win.WPARAM(sectionFont()), 1)
		}
		if me.init != nil {
			me.fill(*me.init)
		}
		return 0
	})
	btnSave.On().BnClicked(me.save)
	me.wnd.On().WmCommand(uint16(co.ID_OK), co.CMD_MENU, me.save)
	btnCancel.On().BnClicked(me.close)
	me.wnd.On().WmCommand(uint16(co.ID_CANCEL), co.CMD_MENU, me.close)

	for _, e := range []*ui.Edit{me.issuer, me.name, me.secret} {
		e.On().EnChange(func() { me.maybeURI(e) })
	}
	me.wnd.ShowModal()
}

// maybeURI fills the form when an otpauth:// link is pasted into a field.
func (me *addDialog) maybeURI(e *ui.Edit) {
	if me.busy {
		return
	}
	text := strings.TrimSpace(e.Text())
	if !strings.HasPrefix(strings.ToLower(text), "otpauth://") {
		return
	}
	a, err := parseOtpauth(text)
	if err != nil {
		me.busy = true
		e.SetText("")
		me.busy = false
		e.ShowBalloonTip("Invalid link", err.Error(), co.TTI_ERROR)
		return
	}
	me.fill(a)
}

func (me *addDialog) fill(a Account) {
	me.busy = true
	defer func() { me.busy = false }()

	me.issuer.SetText(a.Issuer)
	me.name.SetText(a.AccountName)
	me.secret.SetText(a.Secret)
	me.algorithm.SelectIndex(max(0, slices.Index(Algorithms, a.Algorithm)))
	me.digits.SelectIndex(max(0, slices.Index(DigitsList, a.Digits)))
	me.period.SelectIndex(max(0, slices.Index(Periods, a.Period)))
	if !validSecret(a.Secret) {
		me.secret.Focus()
		me.secret.ShowBalloonTip("Secret", "Invalid secret provided", co.TTI_ERROR)
	}
}

func (me *addDialog) save() {
	a := Account{
		Issuer:      strings.TrimSpace(me.issuer.Text()),
		AccountName: strings.TrimSpace(me.name.Text()),
		Secret:      normalizeSecret(me.secret.Text()),
		Algorithm:   Algorithms[max(0, me.algorithm.SelectedIndex())],
		Digits:      DigitsList[max(0, me.digits.SelectedIndex())],
		Period:      Periods[max(0, me.period.SelectedIndex())],
	}
	if !validSecret(a.Secret) {
		me.secret.Focus()
		me.secret.ShowBalloonTip("Secret", "Invalid secret provided", co.TTI_ERROR)
		return
	}
	me.close()
	me.app.addAccounts(a)
}

func (me *addDialog) close() {
	me.wnd.Hwnd().SendMessage(co.WM_CLOSE, 0, 0)
}
