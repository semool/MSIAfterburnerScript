package main

import (
	"log"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"runtime"

	"MSIAfterburnerProfileSwitcher/config"
	"MSIAfterburnerProfileSwitcher/logger"
	"MSIAfterburnerProfileSwitcher/trayicon"
	"MSIAfterburnerProfileSwitcher/watcher"

	"github.com/getlantern/systray"
)

// runAfterburner executes the MSI Afterburner command.
func runAfterburner(exe, arg string) {
	cmd := exec.Command(exe, arg)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to launch Afterburner with profile %s: %v", arg, err)
	} else {
		log.Printf("Successfully applied profile: %s", arg)
	}
}

// checkStateAndApplyProfile is the core logic for determining and applying a profile.
// It now uses the Overrides map in the config as the sole list of targets.
func checkStateAndApplyProfile(cfg *config.Config, currentProfile *string) {
	// The list of targets is now the keys of the Overrides map.
	// The watcher will prioritize the foreground application.
	activeTarget, isActive := watcher.FirstActiveTarget(cfg.Overrides)

	var desiredProfile string
	if isActive {
		profile := cfg.Overrides[activeTarget]
		if profile != "" {
			desiredProfile = profile
		} else {
			desiredProfile = cfg.ProfileOn
		}
	} else {
		desiredProfile = cfg.ProfileOff
	}

	if desiredProfile != *currentProfile {
		if isActive {
			log.Printf("State change! Target found: '%s', Desired profile: %s", activeTarget, desiredProfile)
		} else {
			log.Printf("State change! Target found: '', Desired profile: %s", desiredProfile)
		}
		runAfterburner(cfg.AfterburnerPath, desiredProfile)
		*currentProfile = desiredProfile
	}
}

// startPollingMode runs the application by checking for targets on a timer.
func startPollingMode(cfg config.Config) {
	log.Println("Starting in Polling Mode")
	var currentProfile string
	checkStateAndApplyProfile(&cfg, &currentProfile)
	ticker := time.NewTicker(time.Duration(cfg.DelaySeconds) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		reloadedCfg := config.Load()
		cfg.ProfileOn = reloadedCfg.ProfileOn
		cfg.ProfileOff = reloadedCfg.ProfileOff
		cfg.Overrides = reloadedCfg.Overrides
		cfg.AfterburnerPath = reloadedCfg.AfterburnerPath
		checkStateAndApplyProfile(&cfg, &currentProfile)
	}
}

// startEventMode runs the application by listening for system events.
func startEventMode(cfg config.Config) {
	log.Println("Starting in Event-Driven Mode")
	var currentProfile string
	eventHandler := func() {
		reloadedCfg := config.Load()
		cfg.ProfileOn = reloadedCfg.ProfileOn
		cfg.ProfileOff = reloadedCfg.ProfileOff
		cfg.Overrides = reloadedCfg.Overrides
		cfg.AfterburnerPath = reloadedCfg.AfterburnerPath
		checkStateAndApplyProfile(&cfg, &currentProfile)
	}
	eventHandler()
	watcher.StartEventWatcher(eventHandler)
	select {}
}

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	//trayicon.HideConsole()

	systray.SetIcon(trayicon.IconData)
	systray.SetTitle("MSI Afterburner Profile Switcher")
	systray.SetTooltip("MSI Afterburner Profile Switcher is running")
	//sConsole := systray.AddMenuItem("Toogle Console", "Toogle Console")
	mLog := systray.AddMenuItem("Show Log", "Open Log Window")
	mQuit := systray.AddMenuItem("Quit", "Quit this app")
	go func() {
		for {
			select {
			//case <-sConsole.ClickedCh:
			//	trayicon.ToggleConsole()
			case <-mLog.ClickedCh:
				logger.OpenOrFocusLogWindow()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()

	logger.InitLogger()
	log.Println("MSI Afterburner Profile Switcher started")

	affinity()

	cfg := config.Load()
	log.Println("Configuration succesfully loaded")

	switch strings.ToLower(cfg.MonitoringMode) {
	case "poll":
		startPollingMode(cfg)
	case "event":
		startEventMode(cfg)
	default:
		log.Fatalf("Invalid monitoring_mode %q in config.json", cfg.MonitoringMode)
	}
}

func affinity() {
	// Run only on the first 4 Cores, mostly Performance Cores
	hProcess := syscall.Handle(^uintptr(0))
	ret, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("SetProcessAffinityMask").Call(uintptr(hProcess), 0xF)
	if ret == 0 {
		log.Fatalf("Error while setting Affinity:", err)
		return
	}
	runtime.GOMAXPROCS(4)
	log.Println("CPU Affinity is set to Cores 0-3")
}

func onExit() {
	//showConsole()
}
