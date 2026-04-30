package tray

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/Sora378/codingplantracker/internal/accounts"
	"github.com/Sora378/codingplantracker/internal/codex"
	"github.com/Sora378/codingplantracker/internal/config"
	"github.com/Sora378/codingplantracker/internal/models"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

//go:embed icon.png
var iconData []byte

const (
	refreshInterval     = 30 * time.Second
	maxLoginAPIKeyBytes = 8 << 10
)

type quotaState struct {
	Profile      accounts.Profile
	LoginStatus  string
	CodexUsage   *codex.Usage
	MiniMaxUsage *models.CurrentUsage
	UpdatedAt    time.Time
	Error        string
	Refreshing   bool
}

type dashboardWidgets struct {
	title       *canvas.Text
	subtitle    *canvas.Text
	account     *canvas.Text
	loginStatus *canvas.Text
	plan        *canvas.Text
	window5H    quotaWidgets
	week        quotaWidgets
	status      *canvas.Text
	theme       *widget.Button
	refresh     *widget.Button
	manage      *widget.Button
}

type quotaWidgets struct {
	title    *canvas.Text
	caption  *canvas.Text
	percent  *canvas.Text
	reset    *canvas.Text
	bg       *canvas.Rectangle
	segments []*canvas.Rectangle
}

type cpqApp struct {
	fyneApp fyne.App
	window  fyne.Window
	manager *accounts.Manager

	mu    sync.Mutex
	state quotaState

	menu        *fyne.Menu
	statusItem  *fyne.MenuItem
	accountItem *fyne.MenuItem
	switchItem  *fyne.MenuItem
	fiveHItem   *fyne.MenuItem
	weekItem    *fyne.MenuItem
	updateItem  *fyne.MenuItem
	themeItem   *fyne.MenuItem
	refreshing  bool
	darkMode    bool

	dashboard dashboardWidgets
}

func RunTray() {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	_ = accounts.MigrateLegacyMiniMax(cfg)
	manager := accounts.NewManager(cfg)
	_ = manager.Config().Save()
	a := &cpqApp{
		fyneApp: app.NewWithID("com.sora378.coplanage"),
		manager: manager,
		state: quotaState{
			LoginStatus: "Checking...",
			Profile:     manager.ActiveProfile(),
		},
		darkMode: true,
	}
	a.fyneApp.SetIcon(fyne.NewStaticResource("cpq-icon.png", iconData))
	a.applyTheme()
	a.buildWindow()
	a.buildTray()
	a.fyneApp.Lifecycle().SetOnStarted(func() {
		a.startAutoRefresh()
		a.refresh()
	})
	a.window.Show()
	a.fyneApp.Run()
}

func (a *cpqApp) buildWindow() {
	w := a.fyneApp.NewWindow("CPQ")
	w.Resize(fyne.NewSize(440, 265))
	w.SetCloseIntercept(func() {
		w.Hide()
	})

	a.dashboard.title = newCanvasText("CPQ", 18, true)
	a.dashboard.subtitle = newCanvasText("Coding plan quota", 13, false)
	a.dashboard.account = newCanvasText("Account: --", 13, false)
	a.dashboard.loginStatus = newCanvasText("Checking", 14, false)
	a.dashboard.plan = newCanvasText("Plan: --", 14, false)
	a.dashboard.status = newCanvasText("Last updated: --", 13, false)
	a.dashboard.theme = widget.NewButton("Light", a.toggleTheme)
	a.dashboard.theme.Importance = widget.LowImportance
	a.dashboard.refresh = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), a.refresh)
	a.dashboard.refresh.Importance = widget.LowImportance
	a.dashboard.manage = widget.NewButton("Accounts", a.showAccountManager)
	a.dashboard.manage.Importance = widget.LowImportance

	a.dashboard.window5H = newQuotaWidgets()
	a.dashboard.week = newQuotaWidgets()

	header := container.NewBorder(
		nil,
		nil,
		nil,
		container.NewHBox(a.dashboard.manage, a.dashboard.theme, a.dashboard.refresh),
		container.NewVBox(a.dashboard.title, a.dashboard.subtitle),
	)
	accountRow := container.NewBorder(nil, nil, a.dashboard.account, nil)
	statusRow := container.NewBorder(nil, nil, a.dashboard.loginStatus, a.dashboard.plan)
	quotaRow := container.NewGridWithColumns(2,
		newQuotaPanel("5H Window", "short burst", a.dashboard.window5H),
		newQuotaPanel("Weekly", "long runway", a.dashboard.week),
	)
	content := container.NewVBox(
		header,
		widget.NewSeparator(),
		accountRow,
		statusRow,
		quotaRow,
		widget.NewSeparator(),
		a.dashboard.status,
	)
	w.SetContent(container.NewPadded(content))
	a.paintStaticColors()
	a.window = w
}

func newCanvasText(text string, size float32, bold bool) *canvas.Text {
	t := canvas.NewText(text, color.White)
	t.TextSize = size
	t.TextStyle = fyne.TextStyle{Bold: bold}
	return t
}

func newQuotaWidgets() quotaWidgets {
	title := newCanvasText("", 14, true)
	caption := newCanvasText("", 12, false)
	percent := newCanvasText("--%", 18, true)
	reset := newCanvasText("resets --", 12, false)
	bg := canvas.NewRectangle(darkPanelColor())
	segments := make([]*canvas.Rectangle, 24)
	for i := range segments {
		segments[i] = canvas.NewRectangle(darkInactiveSegmentColor())
		segments[i].SetMinSize(fyne.NewSize(8, 12))
	}
	return quotaWidgets{title: title, caption: caption, percent: percent, reset: reset, bg: bg, segments: segments}
}

func newQuotaPanel(title, caption string, quota quotaWidgets) fyne.CanvasObject {
	quota.title.Text = title
	quota.caption.Text = caption
	track := container.NewGridWithColumns(len(quota.segments), rectObjects(quota.segments)...)
	body := container.NewVBox(
		container.NewBorder(nil, nil, container.NewVBox(quota.title, quota.caption), quota.percent),
		layout.NewSpacer(),
		track,
		quota.reset,
	)
	return container.NewMax(quota.bg, container.NewPadded(body))
}

func rectObjects(rects []*canvas.Rectangle) []fyne.CanvasObject {
	objects := make([]fyne.CanvasObject, len(rects))
	for i, rect := range rects {
		objects[i] = rect
	}
	return objects
}

func (a *cpqApp) buildTray() {
	a.accountItem = fyne.NewMenuItem("Account: --", nil)
	a.accountItem.Disabled = true
	a.statusItem = fyne.NewMenuItem("Status: Checking...", nil)
	a.statusItem.Disabled = true
	a.fiveHItem = fyne.NewMenuItem("5H used: --%", nil)
	a.fiveHItem.Disabled = true
	a.weekItem = fyne.NewMenuItem("Week used: --%", nil)
	a.weekItem.Disabled = true
	a.updateItem = fyne.NewMenuItem("Updated: --", nil)
	a.updateItem.Disabled = true
	a.themeItem = fyne.NewMenuItem("Switch to Light Mode", a.toggleTheme)
	a.switchItem = fyne.NewMenuItem("Switch Account", nil)
	a.switchItem.ChildMenu = fyne.NewMenu("Switch Account")

	openItem := fyne.NewMenuItem("Open Dashboard", func() {
		a.window.Show()
		a.window.RequestFocus()
	})
	refreshItem := fyne.NewMenuItem("Refresh", a.refresh)
	manageItem := fyne.NewMenuItem("Manage Accounts", a.showAccountManager)
	quitItem := fyne.NewMenuItem("Quit", func() {
		a.fyneApp.Quit()
	})
	quitItem.IsQuit = true

	a.menu = fyne.NewMenu("CPQ",
		openItem,
		fyne.NewMenuItemSeparator(),
		a.accountItem,
		a.statusItem,
		a.fiveHItem,
		a.weekItem,
		a.updateItem,
		fyne.NewMenuItemSeparator(),
		a.switchItem,
		manageItem,
		a.themeItem,
		refreshItem,
		quitItem,
	)

	if desk, ok := a.fyneApp.(desktop.App); ok {
		desk.SetSystemTrayMenu(a.menu)
		desk.SetSystemTrayWindow(a.window)
		desk.SetSystemTrayIcon(fyne.NewStaticResource("cpq-icon.png", iconData))
	}
}

func (a *cpqApp) toggleTheme() {
	a.darkMode = !a.darkMode
	a.applyTheme()
	a.paintStaticColors()
	a.mu.Lock()
	state := a.state
	a.mu.Unlock()
	a.render(state)
}

func (a *cpqApp) rebuildSwitchMenu() {
	if a.switchItem == nil {
		return
	}
	items := make([]*fyne.MenuItem, 0, len(a.manager.Profiles()))
	active := a.manager.ActiveProfileID()
	for _, profile := range a.manager.Profiles() {
		profile := profile
		label := formatProfileName(profile)
		if profile.ID == active {
			label = "Active: " + label
		}
		items = append(items, fyne.NewMenuItem(label, func() {
			if err := a.manager.Switch(profile.ID); err != nil {
				a.showError("Switch failed", err)
				return
			}
			a.refresh()
		}))
	}
	if len(items) == 0 {
		empty := fyne.NewMenuItem("No accounts", nil)
		empty.Disabled = true
		items = append(items, empty)
	}
	a.switchItem.ChildMenu = fyne.NewMenu("Switch Account", items...)
}

func (a *cpqApp) showAccountManager() {
	w := a.fyneApp.NewWindow("CPQ Accounts")
	w.Resize(fyne.NewSize(560, 360))
	list := container.NewVBox()
	status := newCanvasText("", 12, false)
	status.Color = mutedTextColor(a.darkMode)

	var rebuild func()
	rebuild = func() {
		list.Objects = nil
		active := a.manager.ActiveProfileID()
		for _, profile := range a.manager.Profiles() {
			profile := profile
			marker := " "
			if profile.ID == active {
				marker = "*"
			}
			name := newCanvasText(fmt.Sprintf("%s %s", marker, formatProfileName(profile)), 14, true)
			name.Color = primaryTextColor(a.darkMode)
			meta := newCanvasText(profileMeta(profile), 11, false)
			meta.Color = mutedTextColor(a.darkMode)
			switchButton := widget.NewButton("Switch", func() {
				if err := a.manager.Switch(profile.ID); err != nil {
					setCanvasText(status, "Switch failed: "+err.Error())
					return
				}
				setCanvasText(status, "Switched to "+formatProfileName(profile))
				rebuild()
				a.refresh()
			})
			logoutButton := widget.NewButton("Logout", func() {
				if err := a.manager.Logout(profile.ID); err != nil {
					setCanvasText(status, "Logout failed: "+err.Error())
					return
				}
				setCanvasText(status, "Logged out "+formatProfileName(profile))
				a.refresh()
			})
			removeButton := widget.NewButton("Remove", func() {
				dialog.ShowConfirm("Remove Account", "Remove "+formatProfileName(profile)+"?", func(ok bool) {
					if !ok {
						return
					}
					if err := a.manager.Remove(profile.ID); err != nil {
						setCanvasText(status, "Remove failed: "+err.Error())
						return
					}
					setCanvasText(status, "Removed account")
					rebuild()
					a.refresh()
				}, w)
			})
			if profile.ID == "codex-default" {
				removeButton.Disable()
			}
			row := container.NewBorder(nil, nil, container.NewVBox(name, meta), container.NewHBox(switchButton, logoutButton, removeButton))
			list.Add(container.NewVBox(row, widget.NewSeparator()))
		}
		list.Refresh()
	}

	addCodex := widget.NewButton("Add Codex", func() {
		name := widget.NewEntry()
		name.SetPlaceHolder("Personal")
		home := widget.NewEntry()
		home.SetPlaceHolder("optional CODEX_HOME")
		dialog.ShowForm("Add Codex Account", "Add", "Cancel", []*widget.FormItem{
			widget.NewFormItem("Name", name),
			widget.NewFormItem("Codex home", home),
		}, func(ok bool) {
			if !ok {
				return
			}
			if _, err := a.manager.AddCodex(name.Text, home.Text); err != nil {
				setCanvasText(status, "Add Codex failed: "+err.Error())
				return
			}
			setCanvasText(status, "Codex account added")
			rebuild()
		}, w)
	})
	addMiniMax := widget.NewButton("Add MiniMax", func() {
		name := widget.NewEntry()
		name.SetPlaceHolder("Work")
		region := widget.NewSelect([]string{"global", "china"}, nil)
		region.SetSelected("global")
		apiKey := widget.NewPasswordEntry()
		validate := widget.NewCheck("Validate key before saving", nil)
		validate.SetChecked(true)
		dialog.ShowForm("Add MiniMax Account", "Add", "Cancel", []*widget.FormItem{
			widget.NewFormItem("Name", name),
			widget.NewFormItem("Region", region),
			widget.NewFormItem("API key", apiKey),
			widget.NewFormItem("", validate),
		}, func(ok bool) {
			if !ok {
				return
			}
			setCanvasText(status, "Adding MiniMax account...")
			go func() {
				_, err := a.manager.AddMiniMax(context.Background(), name.Text, region.Selected, apiKey.Text, validate.Checked)
				fyne.Do(func() {
					if err != nil {
						setCanvasText(status, "Add MiniMax failed: "+err.Error())
						return
					}
					setCanvasText(status, "MiniMax account added")
					rebuild()
					a.refresh()
				})
			}()
		}, w)
	})
	refresh := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), a.refresh)
	header := container.NewBorder(nil, nil, newCanvasText("Accounts", 18, true), container.NewHBox(addCodex, addMiniMax, refresh))
	w.SetContent(container.NewPadded(container.NewVBox(header, widget.NewSeparator(), list, status)))
	rebuild()
	w.Show()
}

func (a *cpqApp) showError(title string, err error) {
	if err == nil {
		return
	}
	dialog.ShowError(fmt.Errorf("%s: %w", title, err), a.window)
}

func (a *cpqApp) applyTheme() {
	if a.darkMode {
		a.fyneApp.Settings().SetTheme(theme.DarkTheme())
		return
	}
	a.fyneApp.Settings().SetTheme(theme.LightTheme())
}

func (a *cpqApp) paintStaticColors() {
	panel := panelColor(a.darkMode)
	text := primaryTextColor(a.darkMode)
	muted := mutedTextColor(a.darkMode)
	accent := accentTextColor(a.darkMode)
	for _, quota := range []quotaWidgets{a.dashboard.window5H, a.dashboard.week} {
		if quota.bg != nil {
			quota.bg.FillColor = panel
			quota.bg.Refresh()
		}
		quota.title.Color = accent
		quota.caption.Color = muted
		quota.reset.Color = muted
		quota.title.Refresh()
		quota.caption.Refresh()
		quota.reset.Refresh()
	}
	a.dashboard.title.Color = text
	a.dashboard.subtitle.Color = muted
	a.dashboard.account.Color = muted
	a.dashboard.loginStatus.Color = successTextColor(a.darkMode)
	a.dashboard.plan.Color = text
	a.dashboard.status.Color = muted
	for _, textObj := range []*canvas.Text{
		a.dashboard.title,
		a.dashboard.subtitle,
		a.dashboard.account,
		a.dashboard.loginStatus,
		a.dashboard.plan,
		a.dashboard.status,
	} {
		textObj.Refresh()
	}
	a.dashboard.theme.SetText(themeButtonText(a.darkMode))
	if a.themeItem != nil {
		if a.darkMode {
			a.themeItem.Label = "Switch to Light Mode"
		} else {
			a.themeItem.Label = "Switch to Dark Mode"
		}
		if a.menu != nil {
			a.menu.Refresh()
		}
	}
}

func (a *cpqApp) startAutoRefresh() {
	go func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for range ticker.C {
			a.refresh()
		}
	}()
}

func (a *cpqApp) refresh() {
	a.mu.Lock()
	if a.refreshing {
		a.mu.Unlock()
		return
	}
	a.refreshing = true
	a.state.Refreshing = true
	state := a.state
	a.mu.Unlock()
	a.render(state)

	go func() {
		profile := a.manager.ActiveProfile()
		ctx := context.Background()
		next := quotaState{Refreshing: false, Profile: profile}
		status := a.manager.Status(ctx, profile)
		next.LoginStatus = status.Label
		var err error
		switch profile.Provider {
		case accounts.ProviderMiniMax:
			next.MiniMaxUsage, err = a.manager.ReadMiniMaxUsage(ctx, profile)
		default:
			next.CodexUsage, err = a.manager.ReadCodexUsage(ctx, profile)
		}
		if err != nil {
			next.Error = cleanStatusText(err.Error(), 100)
		} else {
			next.UpdatedAt = time.Now()
		}

		a.mu.Lock()
		if err != nil && a.state.Profile.ID == profile.ID {
			next.CodexUsage = a.state.CodexUsage
			next.MiniMaxUsage = a.state.MiniMaxUsage
			next.UpdatedAt = a.state.UpdatedAt
		}
		a.state = next
		a.refreshing = false
		a.mu.Unlock()
		a.render(next)
	}()
}

func (a *cpqApp) render(state quotaState) {
	fyne.Do(func() {
		window5H, weekly := usageWindows(state)

		status := formatProviderStatus(state.Profile, state.LoginStatus)
		fiveH := formatUsageWindow("5H", window5H)
		week := formatUsageWindow("Week", weekly)
		updated := formatUpdatedStatus(state)

		a.accountItem.Label = "Account: " + formatProfileName(state.Profile)
		a.statusItem.Label = status
		a.fiveHItem.Label = fiveH
		a.weekItem.Label = week
		a.updateItem.Label = updated
		a.rebuildSwitchMenu()
		if a.menu != nil {
			a.menu.Refresh()
		}

		setCanvasText(a.dashboard.account, "Account: "+formatProfileName(state.Profile))
		setCanvasText(a.dashboard.loginStatus, status)
		a.dashboard.loginStatus.Color = loginStatusColor(a.darkMode, state)
		a.dashboard.loginStatus.Refresh()
		setCanvasText(a.dashboard.plan, "Plan: "+formatPlan(state))
		setCanvasText(a.dashboard.subtitle, subtitleForProvider(state.Profile))
		updateQuotaWidgets(a.dashboard.window5H, window5H)
		updateQuotaWidgets(a.dashboard.week, weekly)
		setCanvasText(a.dashboard.status, updated)
		if state.Refreshing {
			a.dashboard.refresh.Disable()
		} else {
			a.dashboard.refresh.Enable()
		}
		a.paintStaticColors()

		if desk, ok := a.fyneApp.(desktop.App); ok && window5H != nil {
			desk.SetSystemTrayIcon(fyne.NewStaticResource("cpq-usage.png", renderIcon(usedPercent(window5H.UsedPercent))))
		}
	})
}

func updateQuotaWidgets(quota quotaWidgets, window *codex.Window) {
	if window == nil {
		setCanvasText(quota.percent, "--%")
		setCanvasText(quota.reset, "resets --")
		quota.percent.Color = mutedTextColor(fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantDark)
		quota.percent.Refresh()
		paintSegments(quota.segments, 0, true)
		return
	}
	used := usedPercent(window.UsedPercent)
	setCanvasText(quota.percent, fmt.Sprintf("%.0f%%", used))
	quota.percent.Color = segmentColor(used)
	quota.percent.Refresh()
	setCanvasText(quota.reset, "resets "+formatReset(window))
	paintSegments(quota.segments, used, false)
}

func setCanvasText(text *canvas.Text, value string) {
	text.Text = value
	text.Refresh()
}

func paintSegments(segments []*canvas.Rectangle, used float64, empty bool) {
	active := int((used/100)*float64(len(segments)) + 0.5)
	for i, segment := range segments {
		if !empty && i < active {
			segment.FillColor = segmentColor(used)
		} else {
			segment.FillColor = inactiveSegmentColor(fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantDark)
		}
		segment.Refresh()
	}
}

func panelColor(dark bool) color.Color {
	if dark {
		return darkPanelColor()
	}
	return color.NRGBA{245, 247, 250, 255}
}

func darkPanelColor() color.Color {
	return color.NRGBA{24, 26, 31, 255}
}

func inactiveSegmentColor(dark bool) color.Color {
	if dark {
		return darkInactiveSegmentColor()
	}
	return color.NRGBA{218, 224, 232, 255}
}

func darkInactiveSegmentColor() color.Color {
	return color.NRGBA{42, 46, 54, 255}
}

func primaryTextColor(dark bool) color.Color {
	if dark {
		return color.NRGBA{238, 241, 245, 255}
	}
	return color.NRGBA{31, 35, 42, 255}
}

func mutedTextColor(dark bool) color.Color {
	if dark {
		return color.NRGBA{142, 151, 164, 255}
	}
	return color.NRGBA{91, 101, 116, 255}
}

func accentTextColor(dark bool) color.Color {
	if dark {
		return color.NRGBA{96, 165, 250, 255}
	}
	return color.NRGBA{37, 99, 235, 255}
}

func successTextColor(dark bool) color.Color {
	if dark {
		return color.NRGBA{74, 222, 128, 255}
	}
	return color.NRGBA{22, 163, 74, 255}
}

func dangerTextColor(dark bool) color.Color {
	if dark {
		return color.NRGBA{248, 113, 113, 255}
	}
	return color.NRGBA{220, 38, 38, 255}
}

func warningTextColor(dark bool) color.Color {
	if dark {
		return color.NRGBA{251, 191, 36, 255}
	}
	return color.NRGBA{217, 119, 6, 255}
}

func segmentColor(percent float64) color.Color {
	switch {
	case percent >= 95:
		return color.NRGBA{239, 68, 68, 255}
	case percent >= 80:
		return color.NRGBA{245, 158, 11, 255}
	default:
		return color.NRGBA{34, 197, 94, 255}
	}
}

func loginStatusColor(dark bool, state quotaState) color.Color {
	switch {
	case state.Error != "" && !hasUsage(state):
		return dangerTextColor(dark)
	case state.LoginStatus == "Connected":
		return successTextColor(dark)
	case state.Refreshing:
		return warningTextColor(dark)
	default:
		return mutedTextColor(dark)
	}
}

func themeButtonText(dark bool) string {
	if dark {
		return "Light"
	}
	return "Dark"
}

func formatLoginStatus(status string) string {
	if status == "" {
		status = "Checking"
	}
	return "Codex: " + status
}

func formatProviderStatus(profile accounts.Profile, status string) string {
	if status == "" {
		status = "Checking"
	}
	return providerLabel(profile.Provider) + ": " + status
}

func providerLabel(provider string) string {
	switch provider {
	case accounts.ProviderMiniMax:
		return "MiniMax"
	default:
		return "Codex"
	}
}

func subtitleForProvider(profile accounts.Profile) string {
	return "Coding plan quota"
}

func formatProfileName(profile accounts.Profile) string {
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = providerLabel(profile.Provider)
	}
	return providerLabel(profile.Provider) + " - " + name
}

func profileMeta(profile accounts.Profile) string {
	switch profile.Provider {
	case accounts.ProviderMiniMax:
		region := profile.Region
		if region == "" {
			region = "global"
		}
		return "MiniMax API region: " + region
	case accounts.ProviderCodex:
		if strings.TrimSpace(profile.CodexHome) != "" {
			return "Codex home: " + profile.CodexHome
		}
		return "Uses active Codex CLI session"
	default:
		return profile.Provider
	}
}

func parseCodexLoginStatus(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "WARNING:") {
			continue
		}
		return cleanStatusText(line, 70)
	}
	return "unknown"
}

func cleanStatusText(text string, maxLen int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > maxLen {
		return text[:maxLen]
	}
	return text
}

func splitCodexWindows(primary, secondary *codex.Window) (*codex.Window, *codex.Window) {
	var window5H *codex.Window
	var weekly *codex.Window
	for _, window := range []*codex.Window{primary, secondary} {
		if window == nil || window.WindowDurationMins == nil {
			continue
		}
		switch *window.WindowDurationMins {
		case 300:
			window5H = window
		case 10080:
			weekly = window
		}
	}
	if window5H == nil {
		window5H = primary
	}
	if weekly == nil {
		weekly = secondary
	}
	return window5H, weekly
}

func formatCodexWindow(label string, window *codex.Window) string {
	return formatUsageWindow(label, window)
}

func formatUsageWindow(label string, window *codex.Window) string {
	if window == nil {
		return label + " used: --%"
	}
	used := usedPercent(window.UsedPercent)
	reset := ""
	if window.ResetsAt != nil {
		reset = " [resets " + formatReset(window) + "]"
	}
	return fmt.Sprintf("%s used: %.1f%%%s%s", label, used, usageLabel(used), reset)
}

func formatReset(window *codex.Window) string {
	if window == nil || window.ResetsAt == nil {
		return "--"
	}
	return time.Unix(*window.ResetsAt, 0).Format("Mon 15:04")
}

func formatPlan(state quotaState) string {
	if state.Profile.Provider == accounts.ProviderMiniMax {
		if state.MiniMaxUsage == nil || state.MiniMaxUsage.Plan.Name == "" {
			return "--"
		}
		return state.MiniMaxUsage.Plan.Name
	}
	if state.CodexUsage == nil || state.CodexUsage.PlanType == "" {
		return "--"
	}
	return state.CodexUsage.PlanType
}

func formatUpdatedStatus(state quotaState) string {
	switch {
	case state.Refreshing && !hasUsage(state):
		return "Last updated: loading"
	case state.Error != "" && !hasUsage(state):
		return "Error: " + state.Error
	case state.Error != "":
		return "Last updated: " + state.UpdatedAt.Format("15:04:05") + " (stale, " + state.Error + ")"
	case !state.UpdatedAt.IsZero():
		return "Last updated: " + state.UpdatedAt.Format("15:04:05")
	default:
		return "Last updated: --"
	}
}

func hasUsage(state quotaState) bool {
	return state.CodexUsage != nil || state.MiniMaxUsage != nil
}

func usageWindows(state quotaState) (*codex.Window, *codex.Window) {
	if state.Profile.Provider == accounts.ProviderMiniMax {
		return miniMaxWindows(state.MiniMaxUsage)
	}
	if state.CodexUsage == nil {
		return splitCodexWindows(nil, nil)
	}
	return splitCodexWindows(state.CodexUsage.Primary, state.CodexUsage.Secondary)
}

func miniMaxWindows(usage *models.CurrentUsage) (*codex.Window, *codex.Window) {
	if usage == nil {
		return nil, nil
	}
	windowMins := 300
	weekMins := 10080
	windowReset := usage.WindowEndUnixMs / 1000
	weekReset := int64(0)
	if !usage.LastUpdated.IsZero() {
		nextWeek := usage.LastUpdated.Add(7 * 24 * time.Hour).Unix()
		weekReset = nextWeek
	}
	window := &codex.Window{
		UsedPercent:        usage.WindowPercentUsed,
		WindowDurationMins: &windowMins,
		ResetsAt:           &windowReset,
	}
	weekly := &codex.Window{
		UsedPercent:        usage.WeeklyPercentUsed,
		WindowDurationMins: &weekMins,
	}
	if weekReset != 0 {
		weekly.ResetsAt = &weekReset
	}
	return window, weekly
}

func usedPercent(percent float64) float64 {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func usageLabel(percent float64) string {
	switch {
	case percent >= 95:
		return " CRITICAL"
	case percent >= 80:
		return " WARN"
	default:
		return ""
	}
}

func renderIcon(percent float64) []byte {
	size := 22
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	draw.Draw(img, img.Bounds(), &image.Uniform{color.NRGBA{30, 30, 30, 255}}, image.Point{}, draw.Src)

	pct := fmt.Sprintf("%.0f%%", percent)
	face := basicfont.Face7x13
	textWidth := len(pct) * 7
	x := (size - textWidth) / 2
	if x < 0 {
		x = 0
	}
	d := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{color.NRGBA{255, 255, 255, 255}},
		Face: face,
		Dot:  fixed.P(x, 15),
	}
	d.DrawString(pct)

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

var (
	errInvalidLoginMethod = errors.New("submit requires POST")
	errInvalidLoginNonce  = errors.New("invalid login token")
	errInvalidLoginKey    = errors.New("API key cannot be empty or oversized")
)

func validateLoginSubmission(r *http.Request, expectedNonce string) (string, error) {
	if r.Method != http.MethodPost {
		return "", errInvalidLoginMethod
	}
	r.Body = http.MaxBytesReader(nil, r.Body, maxLoginAPIKeyBytes)
	if err := r.ParseForm(); err != nil {
		return "", errInvalidLoginKey
	}
	if r.PostFormValue("nonce") != expectedNonce {
		return "", errInvalidLoginNonce
	}
	apiKey := strings.TrimSpace(r.PostFormValue("apiKey"))
	if apiKey == "" || len(apiKey) > maxLoginAPIKeyBytes {
		return "", errInvalidLoginKey
	}
	return apiKey, nil
}
