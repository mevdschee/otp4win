package main

import (
	"slices"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
)

var (
	themeValues = []string{ThemeLight, ThemeDark, ThemeSystem}
	themeNames  = []string{"Light", "Dark", "System"}
	sortValues  = []string{SortNone, SortIssuer, SortAccountName}
	sortNames   = []string{"Disabled", "Issuer", "Account name"}
)

// showSettings opens open-otp's settings screen. Changes apply immediately.
func showSettings(a *app) {
	const w, h = 360, 330
	wnd := ui.NewModal(a.wnd,
		ui.OptsModal().
			Title("Settings").
			Size(ui.Dpi(w, h)).
			// No WS_CLIPCHILDREN: group boxes rely on the parent painting
			// behind them.
			Style(co.WS_CAPTION|co.WS_SYSMENU|co.WS_BORDER|co.WS_VISIBLE),
	)
	group := func(text string, y, cy int) {
		ui.NewButton(wnd, ui.OptsButton().
			Text(text).
			Position(ui.Dpi(10, y)).Width(ui.DpiX(w-20)).Height(ui.DpiY(cy)).
			CtrlStyle(co.BS_GROUPBOX).WndStyle(co.WS_CHILD|co.WS_VISIBLE))
	}
	label := func(text string, y int) *ui.Static {
		return ui.NewStatic(wnd, ui.OptsStatic().Text(text).Position(ui.Dpi(24, y+3)))
	}
	combo := func(y int, items []string, sel int) *ui.ComboBox {
		return ui.NewComboBox(wnd, ui.OptsComboBox().
			Position(ui.Dpi(150, y)).Width(ui.DpiX(w-150-24)).
			CtrlStyle(co.CBS_DROPDOWNLIST).
			Texts(items...).Select(max(0, sel)))
	}
	check := func(text string, y int, checked bool) *ui.CheckBox {
		st := co.BST_UNCHECKED
		if checked {
			st = co.BST_CHECKED
		}
		return ui.NewCheckBox(wnd, ui.OptsCheckBox().
			Text(text).Position(ui.Dpi(24, y)).Size(ui.Dpi(w-48, 20)).State(st))
	}

	p := a.prefs
	group("Look && Feel", 10, 62)
	label("&Theme", 36)
	cmbTheme := combo(34, themeNames, slices.Index(themeValues, p.Theme))

	group("Codes Management", 84, 192)
	chkConfirm := check("&Confirm codes deletion", 108, p.ConfirmDelete)
	label("S&ort codes by", 140)
	cmbSort := combo(138, sortNames, slices.Index(sortValues, p.SortBy))
	lblReorder := ui.NewStatic(wnd, ui.OptsStatic().
		Text("You can manually reorder items with drag && drop or Ctrl+Up/Down.").
		Position(ui.Dpi(24, 174)).Size(ui.Dpi(w-48, 36)))
	chkHeaders := check("Show group &header", 174, p.ShowHeaders)
	chkNulls := check("&Empty values first", 200, p.NullsFirst)
	chkReversed := check("&Reverse sort order", 226, p.Reversed)

	btnClose := ui.NewButton(wnd, ui.OptsButton().
		Text("Close").
		Position(ui.Dpi(w-10-88, h-42)).Width(ui.DpiX(88)).
		CtrlStyle(co.BS_DEFPUSHBUTTON))

	updateVisibility := func() {
		manual := a.prefs.SortBy == SortNone
		sw := co.SW_SHOW
		hd := co.SW_HIDE
		if manual {
			sw, hd = co.SW_HIDE, co.SW_SHOW
		}
		lblReorder.Hwnd().ShowWindow(hd)
		chkHeaders.Hwnd().ShowWindow(sw)
		chkNulls.Hwnd().ShowWindow(sw)
		chkReversed.Hwnd().ShowWindow(sw)
	}
	apply := func() {
		a.savePrefs()
		a.refresh()
	}

	wnd.On().WmCreate(func(_ ui.WmCreate) int {
		updateVisibility()
		return 0
	})
	cmbTheme.On().CbnSelChange(func() {
		a.prefs.Theme = themeValues[cmbTheme.SelectedIndex()]
		a.savePrefs()
		a.applyTheme()
	})
	chkConfirm.On().BnClicked(func() {
		a.prefs.ConfirmDelete = chkConfirm.IsChecked()
		apply()
	})
	cmbSort.On().CbnSelChange(func() {
		a.prefs.SortBy = sortValues[cmbSort.SelectedIndex()]
		updateVisibility()
		apply()
	})
	chkHeaders.On().BnClicked(func() {
		a.prefs.ShowHeaders = chkHeaders.IsChecked()
		apply()
	})
	chkNulls.On().BnClicked(func() {
		a.prefs.NullsFirst = chkNulls.IsChecked()
		apply()
	})
	chkReversed.On().BnClicked(func() {
		a.prefs.Reversed = chkReversed.IsChecked()
		apply()
	})
	closeWnd := func() { wnd.Hwnd().SendMessage(co.WM_CLOSE, 0, 0) }
	btnClose.On().BnClicked(closeWnd)
	wnd.On().WmCommand(uint16(co.ID_OK), co.CMD_MENU, closeWnd)
	wnd.On().WmCommand(uint16(co.ID_CANCEL), co.CMD_MENU, closeWnd)

	wnd.ShowModal()
}
