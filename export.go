package main

import (
	"fmt"
	"github.com/ProtonMail/ui"
	"github.com/shurcooL/trayhost"
	"regexp"
	"strconv"
	"strings"
)

var (
	sheetCount    int
	progressBar   *ui.ProgressBar
	progressVal   *ui.Label
	exportPadding *ui.Label
	exportEntry   *ExportEntry
	extensions    = []string{".xlsx", ".xls"}
	prompts       = []string{
		//`1. URL: username:password@tcp(ip:port)/db?Charset=utf8`,
		//`2. SQL: select * from user where user_id = ? and name like ?`,
		//`3. Args: 666,tools (If the parameter contains[,] when, use [\,] to avoid this)`,
		//`4. Titles: ID,姓名,年龄... (This is excel sheet column title)`,
		//`5. Sheet: 用户统计 (This is excel sheet name)`,
		`Tips: When multiple Sheets use the same URL, just fill in the URL of the first Sheet`,
		`If you want to paste content, you need to use the right mouse button.`,
	}
)

type Export struct {
	Window   *ExportWindow
	FileName string
	Download string
	Multiple bool // 多个Sheet使用同一个DBConnection
	Sheets   Sheets
}

type Sheets []SingleSheet

type SingleSheet struct {
	URL       string
	SQL       string
	Args      string
	Titles    string
	SheetName string
}

func exportMenu() trayhost.MenuItem {
	return trayhost.MenuItem{
		Title: exportWindow.Title(),
		Handler: func() {
			exportWindow.Show()
		},
	}
}

func exportOnReady(window *ui.Window) {
	exportWindow = &ExportWindow{
		Window: window,
	}
	exportWindow.OnClosing(func(window *ui.Window) bool {
		exportWindow.Hide()
		return false
	})
	exportEntry = &ExportEntry{
		SheetTab:   make(map[int]int),
		BuildEntry: &BuildEntry{},
	}
	mainBox := ui.NewVerticalBox()
	mainBox.SetPadded(true)

	form := ui.NewForm()
	form.SetPadded(true)
	exportEntry.XLSName = ui.NewEntry()
	form.Append(FileName, exportEntry.XLSName, false)

	// SavePath
	defaultDownload := downloadPath()
	savePathBox := ui.NewHorizontalBox()
	savePathBox.SetPadded(true)
	exportEntry.Download = ui.NewEntry()
	exportEntry.Download.SetReadOnly(true)
	exportEntry.Download.SetText(defaultDownload)
	selectBtn := ui.NewButton(Choose)
	selectBtn.OnClicked(func(button *ui.Button) {
		filename := ui.SaveFile(exportWindow.Window)
		if filename == "" {
			filename = defaultDownload
		}
		if strings.HasSuffix(filename, "/"+Untitled) {
			filename = filename[:strings.LastIndex(filename, "/")]
		}
		exportEntry.Download.SetText(filename)
	})
	savePathBox.Append(selectBtn, false)
	savePathBox.Append(exportEntry.Download, true)
	form.Append(Download, savePathBox, false)

	exportEntry.Extension = ui.NewCombobox()
	exportEntry.Extension.Append(extensions[0])
	exportEntry.Extension.Append(extensions[1])
	exportEntry.Extension.SetSelected(0)
	form.Append(Extension, exportEntry.Extension, false)
	mainBox.Append(form, false)

	// Radio Buttons for Same connection URL from checkbox impl
	exportEntry.YesRadio = ui.NewCheckbox(Yes)
	exportEntry.NoRadio = ui.NewCheckbox(No)
	exportEntry.YesRadio.SetChecked(true)
	exportEntry.YesRadio.OnToggled(onMultiChecked)
	exportEntry.NoRadio.OnToggled(onMultiChecked)
	exportEntry.UseOneURL = exportEntry.YesRadio.Checked()
	radioBox := ui.NewHorizontalBox()
	radioBox.SetPadded(true)
	radioBox.Append(ui.NewLabel("Use the same connection URL for multi sheet?"), false)
	radioBox.Append(exportEntry.YesRadio, false)
	radioBox.Append(exportEntry.NoRadio, false)
	mainBox.Append(radioBox, false)

	exportEntry.Tab = ui.NewTab()
	addNewTab()
	exportEntry.Tab.SetMargined(0, true)
	mainBox.Append(exportEntry.Tab, false)

	exportBtnLine := ui.NewHorizontalBox()
	exportBtnLine.SetPadded(true)
	exportBtn := ui.NewButton(ExportBtn)
	exportBtn.OnClicked(onExportBtnClicked)
	exportEntry.ExportBtn = exportBtn
	exportPadding = ui.NewLabel("")
	progressBar = ui.NewProgressBar()
	progressBar.Hide()
	progressVal = ui.NewLabel(" 0% ")
	progressVal.Hide()
	exportBtnLine.Append(exportPadding, true)
	exportBtnLine.Append(progressBar, true)
	exportBtnLine.Append(progressVal, false)
	exportBtnLine.Append(exportBtn, false)
	mainBox.Append(exportBtnLine, false)

	// Prompt Form format
	separator := ui.NewHorizontalSeparator()
	mainBox.Append(separator, false)
	prompt(mainBox)

	exportWindow.SetChild(mainBox)
}

func onMultiChecked(checkbox *ui.Checkbox) {
	if checkbox.Text() == Yes {
		// checked yes
		if checkbox.Checked() {
			exportEntry.NoRadio.SetChecked(false)
			exportEntry.UseOneURL = true
			exportEntry.SQLEntries[0].URL.OnChanged(onFirstURLChanged)
			for _, entry := range exportEntry.SQLEntries[1:] {
				entry.URL.SetReadOnly(true)
				entry.URL.SetText(exportEntry.SQLEntries[0].URL.Text())
			}
		} else {
			checkbox.SetChecked(true)
		}
	} else {
		if checkbox.Checked() {
			exportEntry.YesRadio.SetChecked(false)
			exportEntry.UseOneURL = false
			exportEntry.SQLEntries[0].URL.OnChanged(nil)
			for _, entry := range exportEntry.SQLEntries {
				entry.URL.SetReadOnly(false)
			}
		} else {
			checkbox.SetChecked(true)
		}
	}
}

func prompt(mainBox *ui.Box) {
	for _, p := range prompts {
		label := ui.NewLabel(p)
		box := ui.NewHorizontalBox()
		box.SetPadded(true)
		box.Append(label, false)
		mainBox.Append(box, false)
		exportEntry.PromptLabels = append(exportEntry.PromptLabels, label)
	}
}

func onURLGenBtnClicked(button *ui.Button) {
	host := exportEntry.BuildEntry.Host.Text()
	if !valid(host, "Please enter the correct IP address") {
		return
	}
	port := exportEntry.BuildEntry.Port.Text()
	msg := "Please enter the correct port number"
	if !valid(port, msg) {
		return
	}
	p, err := strconv.Atoi(port)
	if err != nil || p > 65535 {
		showGenErrMsg(msg)
		return
	}
	user := exportEntry.BuildEntry.User.Text()
	if !valid(user, "Please enter the correct MySQL username") {
		return
	}
	pwd := exportEntry.BuildEntry.Pwd.Text()
	if !valid(pwd, "MySQL password cannot be empty") {
		return
	}
	db := exportEntry.BuildEntry.Db.Text()
	if !valid(db, "Please enter the valid Database") {
		return
	}
	var charset string
	for k := range charsets[exportEntry.BuildEntry.Charset.Selected()] {
		charset = k
	}
	url := fmt.Sprintf(URLFormat, user, pwd, host, port, db, charset)
	for _, e := range exportEntry.SQLEntries {
		e.URL.SetText(url)
	}
	closeBuildPanel(button)
}

func closeBuildPanel(button *ui.Button) {
	exportEntry.BuildURLBtn.Show()
	exportEntry.BuildURLWin.Hide()
	exportWindow.Handle()
	exportWindow.SetContentSize(608, 115)
}

func resize(entry *ui.Entry) {
	entry.OnChanged(func(entry *ui.Entry) {
		entry.Text()
		exportWindow.Handle()
		exportWindow.SetContentSize(608, 115)
	})
}

func valid(text, msg string) bool {
	if text == "" {
		showGenErrMsg(msg)
		return false
	}
	return true
}

func showGenErrMsg(msg string) {
	ui.MsgBox(exportWindow.Window, "Generate URL parameter error.", msg)
}

func onBuildURLBtnClicked(button *ui.Button) {
	button.Hide()
	exportEntry.BuildURLWin.Show()
}

func enableExportBtn() {
	exportEntry.ExportBtn.SetText(ExportBtn)
	exportEntry.ExportBtn.Enable()
	exportWindow.Handle()
}

func hidePrompt() {
	for _, label := range exportEntry.PromptLabels {
		label.SetText("")
	}
}

func onExportBtnClicked(button *ui.Button) {
	defer func() {
		if err := recover(); err != nil {
			ui.MsgBoxError(exportWindow.Window,
				"Error generating Excel document.",
				"Error details: "+fmt.Sprintf("error: %v\n", err))
			enableExportBtn()
		}
	}()
	button.Disable()
	hidePrompt()
	fileName := exportEntry.XLSName.Text()
	if fileName == "" {
		fileName = Untitled
	}
	extension := extensions[exportEntry.Extension.Selected()]
	fileName = strings.TrimSpace(fileName) + extension
	download := exportEntry.Download.Text()
	multiple := exportEntry.YesRadio.Checked()
	var sheets Sheets
	var defaultUrl string
	for index, entry := range exportEntry.SQLEntries {
		url := entry.URL.Text()
		if !parameterIsRegex(url, "", "The URL is incorrectly filled, the URL is empty or the format is incorrect, please fill in again") {
			enableExportBtn()
			return
		}
		sql := entry.SQL.Text()
		if !parameterIsRegex(sql, SQLRegex, "The SQL statement cannot be empty, or it is not a query statement, or the SQL syntax is incorrect. Please check and try again") {
			enableExportBtn()
			return
		}
		if index == 0 {
			defaultUrl = url
		}
		if multiple {
			url = defaultUrl
		}
		sheets = append(sheets, SingleSheet{
			URL:       url,
			SQL:       sql,
			Args:      entry.Args.Text(),
			Titles:    entry.Titles.Text(),
			SheetName: entry.SheetName.Text(),
		})
	}
	export := Export{
		Window:   exportWindow,
		FileName: fileName,
		Download: download,
		Multiple: multiple,
		Sheets:   sheets,
	}
	exportWindow.showProgress()
	exporting(export)
}

func parameterIsRegex(text, regex, msg string) bool {
	if !sheetParameterError(text, msg, func() bool {
		if regex != "" {
			sqlRegex := regexp.MustCompile(regex)
			if !sqlRegex.MatchString(text) {
				return false
			}
		}
		return true
	}) {
		return false
	}
	return true
}

func sheetParameterError(text, msg string, checker func() bool) bool {
	if text == "" || (checker != nil && !checker()) {
		ui.MsgBoxError(exportWindow.Window,
			"Sheet parameters are incorrectly filled.",
			"Error details: "+msg)
		return false
	}
	return true
}

func onAddSheetBtnClicked(index int) {
	// Add new TabSheet to Tab
	addNewTab()
	exportEntry.Tab.SetMargined(len(exportEntry.TabEntries)-1, true)
	// AddEntry Button replace to DeleteButton
	btnGrid := exportEntry.TabEntries[exportEntry.Tab.NumPages()-2]
	btnGrid.Delete(0)
	delBtn := ui.NewButton(Delete)
	delBtn.OnClicked(func(button *ui.Button) {
		onTabDeleted(exportEntry.SheetTab[index-1])
	})
	btnGrid.Append(delBtn, 0, 0, 1, 1, false, ui.AlignEnd, false, ui.AlignFill)
}

func onTabDeleted(sheetIndex int) {
	exportEntry.Tab.Delete(sheetIndex)
	exportEntry.TabEntries = append(exportEntry.TabEntries[:sheetIndex], exportEntry.TabEntries[sheetIndex+1:]...)
	exportEntry.SQLEntries = append(exportEntry.SQLEntries[:sheetIndex], exportEntry.SQLEntries[sheetIndex+1:]...)
	temp := make(map[int]int)
	for k, v := range exportEntry.SheetTab {
		if sheetIndex <= k {
			v0 := v - 1
			if v0 >= 0 {
				temp[k] = v0
			}
		} else {
			temp[k] = v
		}
	}
	exportEntry.SheetTab = temp
	exportEntry.DeletedTab += 1
	if exportEntry.Tab.NumPages() == 1 {
		exportEntry.SQLEntries[0].URL.SetReadOnly(false)
	}
}

func addNewTab() {
	exportEntry.Tab.Append("Sheet-"+strconv.Itoa(sheetCount+1), newTabEntry())
}

func newTabEntry() *ui.Box {
	entryBox := ui.NewVerticalBox()
	entryBox.SetPadded(true)
	entry := &SQLEntry{}
	form := ui.NewForm()
	form.SetPadded(true)
	var input *ui.Entry
	input = ui.NewEntry()
	entry.URL = input
	resize(entry.URL)
	length := len(exportEntry.SQLEntries)
	if !exportEntry.UseOneURL || length > 0 {
		input.SetReadOnly(true)
		input.SetText(exportEntry.SQLEntries[length-1].URL.Text())
	} else {
		input.OnChanged(onFirstURLChanged)
	}
	urlLine := ui.NewHorizontalBox()
	urlLine.SetPadded(false)
	urlLine.Append(input, true)
	exportEntry.BuildURLBtn = ui.NewButton(BuildURL)
	exportEntry.BuildURLBtn.OnClicked(onBuildURLBtnClicked)
	urlLine.Append(ui.NewLabel(" "), false)
	urlLine.Append(exportEntry.BuildURLBtn, false)
	// Build MySQL Connection URL
	urlBox := ui.NewVerticalBox()
	urlBox.SetPadded(true)
	exportEntry.BuildURLWin = ui.NewGroup("Build MySQL Connection URL")
	exportEntry.BuildURLWin.SetMargined(true)
	buildForm := ui.NewForm()
	buildForm.SetPadded(true)
	exportEntry.BuildEntry.Host = ui.NewEntry()
	exportEntry.BuildEntry.Host.SetText("127.0.0.1")
	resize(exportEntry.BuildEntry.Host)
	buildForm.Append("Host", exportEntry.BuildEntry.Host, false)
	exportEntry.BuildEntry.Port = ui.NewEntry()
	exportEntry.BuildEntry.Port.SetText("3306")
	resize(exportEntry.BuildEntry.Port)
	buildForm.Append("Port", exportEntry.BuildEntry.Port, false)
	exportEntry.BuildEntry.User = ui.NewEntry()
	exportEntry.BuildEntry.User.SetText("root")
	resize(exportEntry.BuildEntry.User)
	buildForm.Append("User", exportEntry.BuildEntry.User, false)
	exportEntry.BuildEntry.Pwd = ui.NewPasswordEntry()
	resize(exportEntry.BuildEntry.Pwd)
	buildForm.Append("Password", exportEntry.BuildEntry.Pwd, false)
	exportEntry.BuildEntry.Db = ui.NewEntry()
	resize(exportEntry.BuildEntry.Db)
	buildForm.Append("Database", exportEntry.BuildEntry.Db, false)
	exportEntry.BuildEntry.Charset = ui.NewCombobox()
	for _, m := range charsets {
		for k, v := range m {
			exportEntry.BuildEntry.Charset.Append(k + " - " + v)
		}
	}
	exportEntry.BuildEntry.Charset.SetSelected(0)
	buildForm.Append("Charset", exportEntry.BuildEntry.Charset, false)
	urlBox.Append(buildForm, false)
	genBtn := ui.NewButton("Generate")
	genBtn.OnClicked(onURLGenBtnClicked)
	closeBtn := ui.NewButton("Cancel")
	closeBtn.OnClicked(closeBuildPanel)
	padding := ui.NewLabel("")
	buildLine := ui.NewHorizontalBox()
	buildLine.SetPadded(true)
	buildLine.Append(padding, true)
	buildLine.Append(closeBtn, false)
	buildLine.Append(genBtn, false)
	urlBox.Append(buildLine, false)
	exportEntry.BuildURLWin.SetChild(urlBox)
	exportEntry.BuildURLWin.Hide()
	box := ui.NewVerticalBox()
	box.SetPadded(false)
	box.Append(exportEntry.BuildURLWin, false)
	box.Append(urlLine, false)

	form.Append(URL, box, false)
	input = ui.NewEntry()
	entry.SQL = input
	resize(entry.SQL)
	form.Append(SQL, input, false)
	input = ui.NewEntry()
	entry.Args = input
	resize(entry.Args)
	form.Append(Args, input, false)
	input = ui.NewEntry()
	entry.Titles = input
	resize(entry.Titles)
	form.Append(Titles, input, false)
	input = ui.NewEntry()
	entry.SheetName = input
	resize(entry.SheetName)
	form.Append(Sheet, input, false)
	entryBox.Append(form, false)
	addBtnLine := ui.NewGrid()
	addBtnLine.SetPadded(true)
	addBtn := ui.NewButton(AddSheet)
	addBtn.OnClicked(func(button *ui.Button) {
		onAddSheetBtnClicked(sheetCount)
	})
	addBtnLine.Append(addBtn, 0, 0, 1, 1, false, ui.AlignEnd, false, ui.AlignFill)
	entryBox.Append(addBtnLine, false)
	exportEntry.SheetTab[sheetCount] = sheetCount - exportEntry.DeletedTab
	exportEntry.SQLEntries = append(exportEntry.SQLEntries, entry)
	exportEntry.TabEntries = append(exportEntry.TabEntries, addBtnLine)
	sheetCount++
	return entryBox
}

func onFirstURLChanged(entry *ui.Entry) {
	for _, url := range exportEntry.SQLEntries {
		url.URL.SetText(entry.Text())
	}
}

type ExportEntry struct {
	XLSName      *ui.Entry
	SavePath     *ui.Entry
	SQLEntries   []*SQLEntry
	Extension    *ui.Combobox
	TabEntries   []*ui.Grid
	Tab          *ui.Tab
	DeletedTab   int
	UseOneURL    bool
	YesRadio     *ui.Checkbox
	NoRadio      *ui.Checkbox
	SheetTab     map[int]int
	BuildURLWin  *ui.Group
	BuildEntry   *BuildEntry
	BuildURLBtn  *ui.Button
	Download     *ui.Entry
	ExportBtn    *ui.Button
	PromptLabels []*ui.Label
}

type BuildEntry struct {
	Host    *ui.Entry
	Port    *ui.Entry
	User    *ui.Entry
	Pwd     *ui.Entry
	Db      *ui.Entry
	Charset *ui.Combobox
}

type SQLEntry struct {
	URL       *ui.Entry
	SQL       *ui.Entry
	Args      *ui.Entry
	Titles    *ui.Entry
	SheetName *ui.Entry
}

func (e *ExportEntry) Clear() {
	e.SQLEntries = e.SQLEntries[:1]
	e.SQLEntries[0].URL.SetText("")
	e.SQLEntries[0].SQL.SetText("")
	e.SQLEntries[0].Args.SetText("")
	e.SQLEntries[0].Titles.SetText("")
	e.SQLEntries[0].SheetName.SetText("")
	e.TabEntries = e.TabEntries[:1]
	e.DeletedTab = 0
	checkBox(e.YesRadio, true)
	checkBox(e.NoRadio, false)
	set(e.BuildEntry.Host, "127.0.0.1")
	set(e.BuildEntry.Port, "3306")
	set(e.BuildEntry.User, "root")
	set(e.BuildEntry.Pwd, "")
	set(e.BuildEntry.Db, "")
	set(e.SavePath, downloadPath())
	set(e.XLSName, "")
	selected(e.Extension)
	selected(e.BuildEntry.Charset)
	// TODO exportEntry.Tab clear
}

func checkBox(c *ui.Checkbox, checked bool) {
	if c != nil {
		c.SetChecked(checked)
	}
}

func selected(c *ui.Combobox) {
	if c != nil {
		c.SetSelected(0)
	}
}

func set(entry *ui.Entry, val string) {
	if entry != nil {
		entry.SetText(val)
	}
}
