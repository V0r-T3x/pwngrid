package main

import (
	"os"
	"os/signal"
	"runtime/pprof"
	"time"

	"github.com/evilsocket/islazy/fs"
	"github.com/evilsocket/islazy/log"
	"github.com/jayofelony/pwngrid/api"
	"github.com/jayofelony/pwngrid/crypto"

	"github.com/jayofelony/pwngrid/models"

	"github.com/jayofelony/pwngrid/version"
	"github.com/joho/godotenv"
)

func cleanup() {
	if cpuProfile != "" {
		log.Info("writing CPU profile to %s ...", cpuProfile)
		pprof.StopCPUProfile()
	}

	if memProfile != "" {
		log.Info("writing memory profile to %s ...", memProfile)
		f, err := os.Create(memProfile)
		if err != nil {
			log.Fatal("%v", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				panic(err)
			}
		}()
		if err := pprof.WriteHeapProfile(f); err != nil {
			panic(err)
		}
	}
}

func setupCore() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			log.Warning("received signal %v", sig)
			cleanup()
			os.Exit(0)
		}
	}()

	if cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			log.Fatal("%v", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			panic(err)
		}
	}

	if debug {
		log.Level = log.DEBUG
	} else {
		log.Level = log.INFO
	}
	log.OnFatal = log.ExitOnFatal
}

func waitForKeys() {
	privPath := crypto.PrivatePath(keysPath)
	for {
		if !fs.Exists(privPath) {
			log.Debug("waiting for %s ...", privPath)
			time.Sleep(1 * time.Second)
		} else {
			// give it a moment to finish disk sync
			time.Sleep(2 * time.Second)
			log.Info("%s found", privPath)
			break
		}
	}
}


func setupDB() {
	if err := godotenv.Load(env); err != nil {
		log.Fatal("%v", err)
	}
	if err := models.Setup(); err != nil {
		if nodb {
			log.Warning("%v", err)
		} else {
			log.Fatal("%v", err)
		}
	}
}

func setupMode() string {
	var err error

	// in case -inbox was not explicitly passed
	if receiver != "" || loop == true || id > 0 {
		inbox = true
	}

	// for inbox actions, set the keys to the default path if empty
	if (whoami || inbox) && keysPath == "" {
		keysPath = "/etc/pwnagotchi/"
	}

	// generate keypair
	if generate {
		if keysPath == "" {
			log.Fatal("no -keys path specified")
		} else if crypto.KeysExist(keysPath) {
			log.Fatal("keypair already exists in %s", keysPath)
		}

		if _, err = crypto.LoadOrCreate(keysPath, 4096); err != nil {
			log.Fatal("error generating RSA keypair: %v", err)
		} else {
			log.Info("keypair saved to %s", keysPath)
		}
		os.Exit(0)
	}

	mode := "peer"
	// if keys have been passed explicitly, or one of the inbox actions
	// has been specified, we're running on the unit
	// if keysPath != "" {
	//    mode = "peer"
	// }

	log.Info("pwngrid v%s starting in %s mode ...", version.Version, mode)

	// wait for keys to be generated
	if wait {
		waitForKeys()
	}
	// load the keys
	if keys, err = crypto.Load(keysPath); err != nil {
		log.Fatal("error while loading keys from %s: %v", keysPath, err)
	}
	// print identity and exit
	if whoami {
		if Endpoint == "https://api.opwngrid.xyz/api/v1" {
			log.Info("https://opwngrid.xyz/search/%s", keys.FingerprintHex)
		} else {
			log.Info("https://pwnagotchi.ai/pwnfile/#!%s", keys.FingerprintHex)
		}
		os.Exit(0)
	}
	// only start mesh signaling if this is not an inbox action

	// set up the proper routes for either server or peer mode
	err, server = api.Setup(keys, peer, router, Endpoint, Hostname)
	if err != nil {
		log.Fatal("%v", err)
	}

	return mode
}
