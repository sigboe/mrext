package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wizzomafizzo/mrext/pkg/config"
	"github.com/wizzomafizzo/mrext/pkg/mister"
	"github.com/wizzomafizzo/mrext/pkg/service"
	"github.com/wizzomafizzo/mrext/pkg/utils"
)

// TODO: offer to enable recents option and reboot
// TODO: compatibility with GameEventHub
//       https://github.com/christopher-roelofs/GameEventHub/blob/main/mister.py
// TODO: hashing functions (including inside zips)
// TODO: create example ini file

func startService(logger *service.Logger, cfg *config.UserConfig) (func() error, error) {
	tr, err := newTracker(logger, cfg)
	if err != nil {
		logger.Error("error starting tracker: %s", err)
		os.Exit(1)
	}

	tr.loadCore()
	if !mister.ActiveGameEnabled() {
		mister.SetActiveGame("")
	}

	watcher, err := startFileWatch(tr)
	if err != nil {
		tr.logger.Error("error starting file watch: %s", err)
		os.Exit(1)
	}

	interval := 0
	if cfg.PlayLog.SaveEvery > 0 {
		interval = cfg.PlayLog.SaveEvery
	}
	tr.startTicker(interval)

	return func() error {
		watcher.Close()
		tr.stopAll()
		return nil
	}, nil
}

func tryAddStartup() error {
	var startup mister.Startup

	err := startup.Load()
	if err != nil {
		return err
	}

	if !startup.Exists("mrext/playlog") {
		if utils.YesOrNoPrompt("PlayLog must be set to run on MiSTer startup. Add it now?") {
			err = startup.AddService("mrext/playlog")
			if err != nil {
				return err
			}

			err = startup.Save()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func main() {
	svcOpt := flag.String("service", "", "manage playlog service (start, stop, restart, status)")
	flag.Parse()

	logger := service.NewLogger("playlog")

	cfg, err := config.LoadUserConfig(config.UserConfig{
		PlayLog: config.PlayLogConfig{
			SaveEvery: 5, // minutes
		},
	})
	if err != nil {
		logger.Error("error loading user config: %s", err)
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}

	svc, err := service.NewService(service.ServiceArgs{
		Name:   "playlog",
		Logger: logger,
		Entry: func() (func() error, error) {
			return startService(logger, &cfg)
		},
	})
	if err != nil {
		logger.Error("error creating service: %s", err)
		fmt.Println("Error creating service:", err)
		os.Exit(1)
	}

	if !mister.RecentsOptionEnabled() {
		logger.Error("recents option not enabled, exiting...")
		fmt.Println("The \"recents\" option must be enabled for playlog to work. Configure it in the MiSTer.ini file and reboot.")
		os.Exit(1)
	}

	svc.ServiceHandler(svcOpt)

	err = tryAddStartup()
	if err != nil {
		logger.Error("error adding startup: %s", err)
		fmt.Println("Error adding to startup:", err)
	}

	if !svc.Running() {
		err := svc.Start()
		if err != nil {
			logger.Error("error starting service: %s", err)
			fmt.Println("Error starting service:", err)
			os.Exit(1)
		}
	}

	db, err := openPlayLogDb()
	if err != nil {
		logger.Error("error opening db: %s", err)
		fmt.Println("Error opening database:", err)
		os.Exit(1)
	}

	cores, err := db.topCores(10)
	if err != nil {
		logger.Error("error getting top cores: %s", err)
		fmt.Println("Error getting top cores:", err)
		os.Exit(1)
	}
	maxCoreLen := 0
	for _, core := range cores {
		if len(core.name) > maxCoreLen {
			maxCoreLen = len(core.name)
		}
	}

	games, err := db.topGames(10)
	if err != nil {
		logger.Error("error getting top games: %s", err)
		fmt.Println("Error getting top games:", err)
		os.Exit(1)
	}
	maxGameLen := 0
	for _, game := range games {
		if len(game.name) > maxGameLen {
			maxGameLen = len(game.name)
		}
	}

	fmt.Println("Top played cores:")
	// TODO: convert names using names.txt
	for _, core := range cores {
		hours := core.time / 3600
		minutes := (core.time % 3600) / 60
		fmt.Printf("%-*s  %dh %dm\n", maxCoreLen, core.name, hours, minutes)
	}
	fmt.Println()
	fmt.Println("Top played games:")
	for _, game := range games {
		hours := game.time / 3600
		minutes := (game.time % 3600) / 60
		fmt.Printf("%-*s  %dh %dm\n", maxGameLen, game.name, hours, minutes)
	}
}
