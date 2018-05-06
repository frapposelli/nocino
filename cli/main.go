package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jamiealquiza/envy"
	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
	"gopkg.in/telegram-bot-api.v4"

	"github.com/frapposelli/nocino/pkg/gif"
	"github.com/frapposelli/nocino/pkg/handler"
	"github.com/frapposelli/nocino/pkg/markov"
	"github.com/frapposelli/nocino/pkg/nocino"
)

var (
	numw       int
	plen       int
	tgtoken    string
	state      string
	trustedIDs string
	gifstore   string
	gifmaxsize int
	checkpoint time.Duration
	mchain     *markov.Chain
	gifdb      *gif.GIFDB
	debug      bool
	version    = "dev"
	date       = "unknown"
)

const banner = `  [~]
  |=|
.-' '-.
|-----|  mm   m  mmmm    mmm  mmmmm  mm   m  mmmm
| ~~~ |  #"m  # m"  "m m"   "   #    #"m  # m"  "m
| ~~~ |  # #m # #    # #        #    # #m # #    #
| ~~~ |  #  # # #    # #        #    #  # # #    #
|-----|  #   ##  #mm#   "mmm" mm#mm  #   ##  #mm#
'-----'`

var log = logrus.New()

func init() {
	rand.Seed(time.Now().UTC().UnixNano())

	customFormatter := new(prefixed.TextFormatter)
	customFormatter.FullTimestamp = true
	customFormatter.TimestampFormat = "2006-01-02T15:04:05"
	log.Formatter = customFormatter

	exe, err := os.Executable()
	if err != nil {
		log.Panicln("Cannot find EXE location, panicking")
	}

	flag.IntVar(&numw, "numw", 25, "maximum number of words for the markov chain")
	flag.IntVar(&plen, "plen", 2, "chain prefix length")
	flag.StringVar(&state, "state", fmt.Sprintf("%s/nocino.state.gz", filepath.Dir(exe)), "state file for nocino")
	flag.StringVar(&tgtoken, "token", "", "telegram bot token")
	flag.StringVar(&gifstore, "gifstore", fmt.Sprintf("%s/gifs", filepath.Dir(exe)), "path to store GIFs")
	flag.IntVar(&gifmaxsize, "gifmax", 1048576, "max GIF size in bytes")
	flag.StringVar(&trustedIDs, "trustedids", "", "trusted ids separated by comma")
	flag.DurationVar(&checkpoint, "checkpoint", 60*time.Second, "checkpoint interval for state file in seconds")
	flag.BoolVar(&debug, "debug", false, "print debug")

}

func main() {

	// Print Banner
	for _, b := range strings.Split(fmt.Sprintf("\n%s  version: %s built on %s\n", banner, version, date), "\n") {
		log.Infoln(b)
	}

	// Parse ENV Vars
	envy.Parse("NOCINO")
	flag.Parse()

	if debug {
		log.Level = logrus.DebugLevel
	}

	// Initialize Markov Chain
	mchain = markov.NewChain(plen, log)
	mchain.ReadState(state)
	// state file save ticker
	mchain.RunStateSaveTicker(checkpoint, state)

	// Initialize GIF store and DB
	gifdb = gif.NewGIFDB(gifstore, log)
	gifdb.ReadList()

	n := nocino.NewNocino(tgtoken, trustedIDs, numw, plen, gifmaxsize, log)
	n.RunStatsTicker(mchain, gifdb)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := n.API.GetUpdatesChan(u)
	if err != nil {
		n.Log.Fatalf("Error when setting up updates channel: '%s'. Exiting", err.Error())
	}

	// feedback loop
	for update := range updates {
		// handle update
		go func(update tgbotapi.Update) {
			if update.Message == nil || (update.Message.Text == "" && update.Message.Document == nil) {
				return
			}

			h := handler.NewHandler(n, update, mchain, gifdb)
			if err := h.Handle(); err != nil {
				n.Log.Errorf("Error when handling incoming message: '%s'", err.Error())
			}
		}(update)
	}
}
