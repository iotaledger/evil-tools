package interactive

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/evilwallet"
	"github.com/iotaledger/evil-tools/models"
	"github.com/iotaledger/evil-tools/programs"
	"github.com/iotaledger/evil-tools/spammer"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota.go/v4/nodeclient"
)

const (
	faucetFundsCheck   = time.Minute / 12
	maxConcurrentSpams = 5
	lastSpamsShowed    = 15
	configFilename     = "interactive_config.json"
)

const (
	AnswerEnable  = "enable"
	AnswerDisable = "disable"
)

var (
	faucetTicker   *time.Ticker
	printer        *Printer
	minSpamOutputs int
)

type config struct {
	duration   time.Duration
	timeUnit   time.Duration
	clientURLs map[string]types.Empty
}

var configJSON = fmt.Sprintf(`{
	"webAPI": ["http://localhost:8080","http://localhost:8090"],
	"rate": 2,
	"duration": "20s",
	"timeUnit": "1s",
	"deepEnabled": false,
	"reuseEnabled": true,
	"scenario": "%s",
	"autoRequestingEnabled": false,
	"autoRequestingAmount": "100",
	"useRateSetter": true
}`, spammer.TypeTx)

var defaultConfig = models.Config{
	Rate:                 2,
	Deep:                 false,
	Reuse:                true,
	Scenario:             spammer.TypeTx,
	AutoRequesting:       false,
	AutoRequestingAmount: "100",
	UseRateSetter:        true,
}

var defaultInteractiveConfig = config{
	clientURLs: map[string]types.Empty{
		"http://localhost:8050": types.Void,
		"http://localhost:8060": types.Void,
	},
	duration: 20 * time.Second,
	timeUnit: time.Second,
}

const (
	requestAmount100 = "100"
	requestAmount10k = "10000"
)

// region survey selections  ///////////////////////////////////////////////////////////////////////////////////////////////////////

type action int

const (
	actionWalletDetails action = iota
	actionPrepareFunds
	actionSpamMenu
	actionCurrent
	actionHistory
	actionSettings
	shutdown
)

var actions = []string{"Evil wallet details", "Prepare faucet funds", "New spam", "Active spammers", "Spam history", "Settings", "Close"}

const (
	spamScenario = "Change scenario"
	spamType     = "Update spam options"
	spamDetails  = "Update spam rate and duration"
	startSpam    = "Start the spammer"
	back         = "Go back"
)

var spamMenuOptions = []string{startSpam, spamScenario, spamDetails, spamType, back}

const (
	settingPreparation   = "Auto funds requesting"
	settingAddURLs       = "Add client API url"
	settingRemoveURLs    = "Remove client API urls"
	settingUseRateSetter = "Enable/disable rate setter"
)

var settingsMenuOptions = []string{settingPreparation, settingAddURLs, settingRemoveURLs, settingUseRateSetter, back}

const (
	currentSpamRemove = "Cancel spam"
)

var currentSpamOptions = []string{currentSpamRemove, back}

const (
	mpm string = "Minute, rate is [mpm]"
	mps string = "Second, rate is [mps]"
)

var (
	scenarios     = []string{spammer.TypeBlock, spammer.TypeTx, spammer.TypeDs, spammer.TypeBlowball, "conflict-circle", "guava", "orange", "mango", "pear", "lemon", "banana", "kiwi", "peace"}
	confirms      = []string{AnswerEnable, AnswerDisable}
	outputNumbers = []string{"100", "1000", "5000", "cancel"}
	timeUnits     = []string{mpm, mps}
)

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////

// region interactive ///////////////////////////////////////////////////////////////////////////////////////////////////////

func Run() {
	mode := NewInteractiveMode()

	printer = NewPrinter(mode)

	printer.printBanner()
	mode.loadConfig()
	time.Sleep(time.Millisecond * 100)
	configure(mode)
	go mode.runBackgroundTasks()
	mode.menu()

	for {
		select {
		case id := <-mode.spamFinished:
			mode.summarizeSpam(id)
		case <-mode.mainMenu:
			mode.menu()
		case <-mode.shutdown:
			printer.FarewellBlock()
			mode.saveConfigsToFile()
			os.Exit(0)

			return
		}
	}
}

func configure(mode *Mode) {
	faucetTicker = time.NewTicker(faucetFundsCheck)
	switch mode.Config.AutoRequestingAmount {
	case requestAmount100:
		minSpamOutputs = 40
	case requestAmount10k:
		minSpamOutputs = 2000
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////

// region Mode /////////////////////////////////////////////////////////////////////////////////////////////////////////

type Mode struct {
	evilWallet   *evilwallet.EvilWallet
	shutdown     chan types.Empty
	mainMenu     chan types.Empty
	spamFinished chan int
	action       chan action

	nextAction string

	preparingFunds bool

	Config        models.Config
	innerConfig   config
	blkSent       *atomic.Uint64
	txSent        *atomic.Uint64
	scenariosSent *atomic.Uint64

	activeSpammers map[int]*spammer.Spammer
	spammerLog     *models.SpammerLog
	spamMutex      syncutils.Mutex

	stdOutMutex syncutils.Mutex
}

func NewInteractiveMode() *Mode {
	return &Mode{
		evilWallet:   evilwallet.NewEvilWallet(),
		action:       make(chan action),
		shutdown:     make(chan types.Empty),
		mainMenu:     make(chan types.Empty),
		spamFinished: make(chan int),

		Config:        defaultConfig,
		innerConfig:   defaultInteractiveConfig,
		blkSent:       atomic.NewUint64(0),
		txSent:        atomic.NewUint64(0),
		scenariosSent: atomic.NewUint64(0),

		spammerLog:     models.NewSpammerLog(),
		activeSpammers: make(map[int]*spammer.Spammer),
	}
}

func (m *Mode) runBackgroundTasks() {
	for {
		select {
		case <-faucetTicker.C:
			m.prepareFundsIfNeeded()
		case act := <-m.action:
			switch act {
			case actionSpamMenu:
				go m.spamMenu()
			case actionWalletDetails:
				m.walletDetails()
				m.mainMenu <- types.Void
			case actionPrepareFunds:
				m.prepareFunds()
				m.mainMenu <- types.Void
			case actionHistory:
				m.history()
				m.mainMenu <- types.Void
			case actionCurrent:
				go m.currentSpams()
			case actionSettings:
				go m.settingsMenu()
			case shutdown:
				m.shutdown <- types.Void
			}
		}
	}
}

func (m *Mode) walletDetails() {
	m.stdOutMutex.Lock()
	defer m.stdOutMutex.Unlock()

	printer.EvilWalletStatus()
}

func (m *Mode) menu() {
	m.stdOutMutex.Lock()
	defer m.stdOutMutex.Unlock()
	err := survey.AskOne(actionQuestion, &m.nextAction)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	m.onMenuAction()
}

func (m *Mode) onMenuAction() {
	switch m.nextAction {
	case actions[actionWalletDetails]:
		m.action <- actionWalletDetails
	case actions[actionPrepareFunds]:
		m.action <- actionPrepareFunds
	case actions[actionSpamMenu]:
		m.action <- actionSpamMenu
	case actions[actionSettings]:
		m.action <- actionSettings
	case actions[actionCurrent]:
		m.action <- actionCurrent
	case actions[actionHistory]:
		m.action <- actionHistory
	case actions[shutdown]:
		m.action <- shutdown
	}
}

func (m *Mode) prepareFundsIfNeeded() {
	if m.evilWallet.UnspentOutputsLeft(evilwallet.Fresh) < minSpamOutputs {
		if !m.preparingFunds && m.Config.AutoRequesting {
			m.preparingFunds = true
			go func() {
				switch m.Config.AutoRequestingAmount {
				case requestAmount100:
					_ = m.evilWallet.RequestFreshFaucetWallet()
				case requestAmount10k:
					_ = m.evilWallet.RequestFreshBigFaucetWallet()
				}
				m.preparingFunds = false
			}()
		}
	}
}

func (m *Mode) prepareFunds() {
	m.stdOutMutex.Lock()
	defer m.stdOutMutex.Unlock()
	printer.DevNetFundsWarning()

	if m.preparingFunds {
		printer.FundsCurrentlyPreparedWarning()
		return
	}
	if len(m.innerConfig.clientURLs) == 0 {
		printer.NotEnoughClientsWarning(1)
	}
	numToPrepareStr := ""
	err := survey.AskOne(fundsQuestion, &numToPrepareStr)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	switch numToPrepareStr {
	case "100":
		go func() {
			m.preparingFunds = true
			err = m.evilWallet.RequestFreshFaucetWallet()
			m.preparingFunds = false
		}()
	case "1000":
		go func() {
			m.preparingFunds = true
			_ = m.evilWallet.RequestFreshBigFaucetWallet()
			m.preparingFunds = false
		}()
	case "cancel":
		return
	case "5000":
		go func() {
			m.preparingFunds = true
			m.evilWallet.RequestFreshBigFaucetWallets(5)
			m.preparingFunds = false
		}()
	}

	printer.StartedPreparingBlock(numToPrepareStr)
}

func (m *Mode) spamMenu() {
	m.stdOutMutex.Lock()
	defer m.stdOutMutex.Unlock()
	printer.SpammerSettings()
	var submenu string
	err := survey.AskOne(spamMenuQuestion, &submenu)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	m.spamSubMenu(submenu)
}

func (m *Mode) spamSubMenu(menuType string) {
	switch menuType {
	case spamDetails:
		defaultTimeUnit := timeUnitToString(m.innerConfig.duration)
		var spamSurvey spamDetailsSurvey
		err := survey.Ask(spamDetailsQuestions(strconv.Itoa(int(m.innerConfig.duration.Seconds())), strconv.Itoa(m.Config.Rate), defaultTimeUnit), &spamSurvey)
		if err != nil {
			fmt.Println(err.Error())
			m.mainMenu <- types.Void

			return
		}
		m.parseSpamDetails(spamSurvey)

	case spamType:
		var spamSurvey spamTypeSurvey
		err := survey.Ask(spamTypeQuestions(boolToEnable(m.Config.Deep), boolToEnable(m.Config.Reuse)), &spamSurvey)
		if err != nil {
			fmt.Println(err.Error())
			m.mainMenu <- types.Void

			return
		}
		m.parseSpamType(spamSurvey)

	case spamScenario:
		scenario := ""
		err := survey.AskOne(spamScenarioQuestion(m.Config.Scenario), &scenario)
		if err != nil {
			fmt.Println(err.Error())
			m.mainMenu <- types.Void

			return
		}
		m.parseScenario(scenario)

	case startSpam:
		if m.areEnoughFundsAvailable() {
			printer.FundsWarning()
			m.mainMenu <- types.Void

			return
		}
		if len(m.activeSpammers) >= maxConcurrentSpams {
			printer.MaxSpamWarning()
			m.mainMenu <- types.Void

			return
		}
		m.startSpam()

	case back:
		m.mainMenu <- types.Void
		return
	}
	m.action <- actionSpamMenu
}

func (m *Mode) areEnoughFundsAvailable() bool {
	outputsNeeded := m.Config.Rate * int(m.innerConfig.duration.Seconds())
	if m.innerConfig.timeUnit == time.Minute {
		outputsNeeded = int(float64(m.Config.Rate) * m.innerConfig.duration.Minutes())
	}

	return m.evilWallet.UnspentOutputsLeft(evilwallet.Fresh) < outputsNeeded && m.Config.Scenario != spammer.TypeBlock
}

func (m *Mode) startSpam() {
	m.spamMutex.Lock()
	defer m.spamMutex.Unlock()

	var s *spammer.Spammer
	if m.Config.Scenario == spammer.TypeBlock {
		s = programs.SpamBlocks(m.evilWallet, m.Config.Rate, time.Second, m.innerConfig.duration, m.Config.UseRateSetter, "")
	} else {
		scenario, _ := evilwallet.GetScenario(m.Config.Scenario)
		s = programs.SpamNestedConflicts(m.evilWallet, m.Config.Rate, time.Second, m.innerConfig.duration, scenario, m.Config.Deep, m.Config.Reuse, m.Config.UseRateSetter, "")
		if s == nil {
			return
		}
	}
	spamID := m.spammerLog.AddSpam(m.Config)
	m.activeSpammers[spamID] = s
	go func(id int) {
		s.Spam()
		m.spamFinished <- id
	}(spamID)
	printer.SpammerStartedBlock()
}

func (m *Mode) settingsMenu() {
	m.stdOutMutex.Lock()
	defer m.stdOutMutex.Unlock()
	printer.Settings()
	var submenu string
	err := survey.AskOne(settingsQuestion, &submenu)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	m.settingsSubMenu(submenu)
}

func (m *Mode) settingsSubMenu(menuType string) {
	switch menuType {
	case settingPreparation:
		answer := ""
		err := survey.AskOne(autoCreationQuestion, &answer)
		if err != nil {
			fmt.Println(err.Error())
			m.mainMenu <- types.Void

			return
		}
		m.onFundsCreation(answer)

	case settingAddURLs:
		var url string
		err := survey.AskOne(addURLQuestion, &url)
		if err != nil {
			fmt.Println(err.Error())
			m.mainMenu <- types.Void

			return
		}
		m.validateAndAddURL(url)

	case settingRemoveURLs:
		answer := make([]string, 0)
		urlsList := m.urlMapToList()
		err := survey.AskOne(removeURLQuestion(urlsList), &answer)
		if err != nil {
			fmt.Println(err.Error())
			m.mainMenu <- types.Void

			return
		}
		m.removeUrls(answer)

	case settingUseRateSetter:
		answer := ""
		err := survey.AskOne(enableRateSetterQuestion, &answer)
		if err != nil {
			fmt.Println(err.Error())
			m.mainMenu <- types.Void

			return
		}
		m.onEnableRateSetter(answer)

	case back:
		m.mainMenu <- types.Void
		return
	}
	m.action <- actionSettings
}

func (m *Mode) validateAndAddURL(url string) {
	url = "http://" + url
	ok := validateURL(url)
	if !ok {
		printer.URLWarning()
	} else {
		if _, ok := m.innerConfig.clientURLs[url]; ok {
			printer.URLExists()
			return
		}
		m.innerConfig.clientURLs[url] = types.Void
		m.evilWallet.AddClient(url)
	}
}

func (m *Mode) onFundsCreation(answer string) {
	if answer == AnswerEnable {
		m.Config.AutoRequesting = true
		printer.AutoRequestingEnabled()
		m.prepareFundsIfNeeded()
	} else {
		m.Config.AutoRequesting = false
	}
}

func (m *Mode) onEnableRateSetter(answer string) {
	if answer == AnswerEnable {
		m.Config.UseRateSetter = true
		printer.RateSetterEnabled()
	} else {
		m.Config.UseRateSetter = false
	}
}

func (m *Mode) history() {
	m.stdOutMutex.Lock()
	defer m.stdOutMutex.Unlock()
	printer.History()
}

func (m *Mode) currentSpams() {
	m.stdOutMutex.Lock()
	defer m.stdOutMutex.Unlock()

	if len(m.activeSpammers) == 0 {
		printer.Println(printer.colorString("There are no currently running spammers.", "red"), 1)
		fmt.Println("")
		m.mainMenu <- types.Void

		return
	}
	printer.CurrentSpams()
	answer := ""
	err := survey.AskOne(currentMenuQuestion, &answer)
	if err != nil {
		fmt.Println(err.Error())
		m.mainMenu <- types.Void

		return
	}

	m.currentSpamsSubMenu(answer)
}

func (m *Mode) currentSpamsSubMenu(menuType string) {
	switch menuType {
	case currentSpamRemove:
		if len(m.activeSpammers) == 0 {
			printer.NoActiveSpammer()
		} else {
			answer := ""
			err := survey.AskOne(removeSpammer, &answer)
			if err != nil {
				fmt.Println(err.Error())
				m.mainMenu <- types.Void

				return
			}
			m.parseIDToRemove(answer)
		}

		m.action <- actionCurrent

	case back:
		m.mainMenu <- types.Void
		return
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region parsers /////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (m *Mode) parseSpamDetails(details spamDetailsSurvey) {
	d, _ := strconv.Atoi(details.SpamDuration)
	dur := time.Second * time.Duration(d)
	rate, err := strconv.Atoi(details.SpamRate)
	if err != nil {
		return
	}
	switch details.TimeUnit {
	case mpm:
		m.innerConfig.timeUnit = time.Minute
	case mps:
		m.innerConfig.timeUnit = time.Second
	}
	m.Config.Rate = rate
	m.innerConfig.duration = dur
	fmt.Println(details)
}

func (m *Mode) parseSpamType(spamType spamTypeSurvey) {
	deep := enableToBool(spamType.DeepSpamEnabled)
	reuse := enableToBool(spamType.ReuseLaterEnabled)
	m.Config.Deep = deep
	m.Config.Reuse = reuse
}

func (m *Mode) parseScenario(scenario string) {
	m.Config.Scenario = scenario
}

func (m *Mode) removeUrls(urls []string) {
	for _, url := range urls {
		if _, ok := m.innerConfig.clientURLs[url]; ok {
			delete(m.innerConfig.clientURLs, url)
			m.evilWallet.RemoveClient(url)
		}
	}
}

func (m *Mode) urlMapToList() (list []string) {
	for url := range m.innerConfig.clientURLs {
		list = append(list, url)
	}

	return
}

func (m *Mode) parseIDToRemove(answer string) {
	m.spamMutex.Lock()
	defer m.spamMutex.Unlock()

	id, err := strconv.Atoi(answer)
	if err != nil {
		return
	}
	m.summarizeSpam(id)
}

func (m *Mode) summarizeSpam(id int) {
	if s, ok := m.activeSpammers[id]; ok {
		m.updateSentStatistic(s, id)
		m.spammerLog.SetSpamEndTime(id)
		delete(m.activeSpammers, id)
	} else {
		printer.ClientNotFoundWarning(id)
	}
}

func (m *Mode) updateSentStatistic(s *spammer.Spammer, id int) {
	blkSent := s.BlocksSent()
	scenariosCreated := s.BatchesPrepared()
	if m.spammerLog.SpamDetails(id).Scenario == spammer.TypeBlock {
		m.blkSent.Add(blkSent)
	} else {
		m.txSent.Add(blkSent)
	}
	m.scenariosSent.Add(scenariosCreated)
}

// load the config file.
func (m *Mode) loadConfig() {
	// open config file
	file, err := os.Open(configFilename)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}

		//nolint:gosec // users should be able to read the file
		if err = os.WriteFile("config.json", []byte(configJSON), 0o644); err != nil {
			panic(err)
		}
		if file, err = os.Open("config.json"); err != nil {
			panic(err)
		}
	}
	defer file.Close()

	// decode config file
	if err = json.NewDecoder(file).Decode(&m.Config); err != nil {
		panic(err)
	}
	// convert urls array to map
	if len(m.Config.WebAPI) > 0 {
		// rewrite default value
		for url := range m.innerConfig.clientURLs {
			m.evilWallet.RemoveClient(url)
		}
		m.innerConfig.clientURLs = make(map[string]types.Empty)
	}
	for _, url := range m.Config.WebAPI {
		m.innerConfig.clientURLs[url] = types.Void
		m.evilWallet.AddClient(url)
	}
	// parse duration
	d, err := time.ParseDuration(m.Config.Duration)
	if err != nil {
		d = time.Minute
	}
	u, err := time.ParseDuration(m.Config.TimeUnit)
	if err != nil {
		u = time.Second
	}
	m.innerConfig.duration = d
	m.innerConfig.timeUnit = u
}

func (m *Mode) saveConfigsToFile() {
	// open config file
	file, err := os.Open("config.json")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// update client urls
	m.Config.WebAPI = []string{}
	for url := range m.innerConfig.clientURLs {
		m.Config.WebAPI = append(m.Config.WebAPI, url)
	}

	// update duration
	m.Config.Duration = m.innerConfig.duration.String()

	// update time unit
	m.Config.TimeUnit = m.innerConfig.timeUnit.String()

	jsonConfigs, err := json.MarshalIndent(m.Config, "", "    ")
	if err != nil {
		panic(err)
	}
	//nolint:gosec // users should be able to read the file
	if err = os.WriteFile("config.json", jsonConfigs, 0o644); err != nil {
		panic(err)
	}
}

func enableToBool(e string) bool {
	return e == AnswerEnable
}

func boolToEnable(b bool) string {
	if b {
		return AnswerEnable
	}

	return AnswerDisable
}

func validateURL(url string) (ok bool) {
	_, err := nodeclient.New(url)
	if err != nil {
		return
	}

	return true
}

func timeUnitToString(d time.Duration) string {
	durStr := d.String()

	if strings.Contains(durStr, "s") {
		return mps
	}

	return mpm
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region SpammerLog ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
