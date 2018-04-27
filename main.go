package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jamiealquiza/envy"
	"gopkg.in/telegram-bot-api.v4"
)

var (
	numw           int
	plen           int
	tgtoken        string
	state          string
	botUsername    string
	genText        string
	trustedIDs     string
	gifstore       string
	gifmaxsize     int
	answerRequired bool
	elapsed        time.Duration
	checkpoint     time.Duration
	markov         *Chain
	gifdb          *GIFDB
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
	exe, _ := os.Executable()

	flag.IntVar(&numw, "numw", 25, "maximum number of words for the markov chain")
	flag.IntVar(&plen, "plen", 2, "chain prefix length")
	flag.StringVar(&state, "state", "nocino.state.gz", "state file for nocino")
	flag.StringVar(&tgtoken, "token", "", "telegram bot token")
	flag.StringVar(&gifstore, "gifstore", fmt.Sprintf("%s/gifs", filepath.Dir(exe)), "path to store GIFs")
	flag.IntVar(&gifmaxsize, "gifmax", 1048576, "max GIF size in bytes")
	flag.StringVar(&trustedIDs, "trustedids", "", "trusted ids separated by comma")
	flag.DurationVar(&checkpoint, "checkpoint", 60*time.Second, "checkpoint interval for state file in seconds")
}

func main() {
	envy.Parse("NOCINO")
	flag.Parse()
	bot, err := tgbotapi.NewBotAPI(tgtoken)
	if err != nil {
		log.Printf("Cannot log in, exiting...")
		os.Exit(1)
	}

	botUsername = fmt.Sprintf("@%s", bot.Self.UserName)
	log.Printf("Authorized on account %s", botUsername)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := bot.GetUpdatesChan(u)

	markov = NewChain(plen)

	err = markov.ReadState(state)
	if err != nil {
		log.Printf("State file %s not present, creating a new one", state)
	}

	log.Printf("Loaded previous state from '%s' (%d suffixes).", state, len(markov.Chain))

	gifdb = NewGIFDB(gifstore)
	gifdb.ReadList()
	log.Printf("Loaded GIF list from '%s' (%d element/s).", gifdb.Store, len(gifdb.List))

	trustedMap := make(map[int]bool)
	if trustedIDs != "" {
		ids := strings.Split(trustedIDs, ",")
		for i := 0; i < len(ids); i += 1 {
			j, _ := strconv.Atoi(ids[i])
			trustedMap[j] = true
		}
	}

	// state file save ticker
	log.Printf("Starting state save ticker with %s interval", checkpoint.String())
	ticker := time.NewTicker(checkpoint)
	go func() {
		for tick := range ticker.C {
			if err := markov.WriteState(state); err != nil {
				log.Printf("[state] checkpoint failed: %s (in %s)", err.Error(), time.Since(tick).String())
				return
			}
			log.Printf("[state] checkpoint completed, %d suffixes in chain (in %s)", len(markov.Chain), time.Since(tick).String())
		}
	}()

	// feedback loop
	for update := range updates {
		go func() {
			answerRequired = false
			if update.Message == nil || (update.Message.Text == "" && update.Message.Document == nil) {
				return
			}

			// if it's a private message, check against a list of ID
			if update.Message.Chat.Type == "private" {
				if trustedMap[update.Message.From.ID] {
					log.Printf("[%s] Authorized private chat, asking: '%s'", update.Message.From.UserName, update.Message.Text)
					answerRequired = true
				} else {
					// if it's not in the authorized list, do not log
					log.Printf("[%s] Unauthorized private chat, asking: '%s'", update.Message.From.UserName, update.Message.Text)
					return
				}
			}

			// tokenize message
			tokens := strings.Split(update.Message.Text, " ")

			// if it's a reply, check if it's to us, answer back if necessary.
			if update.Message.ReplyToMessage != nil && update.Message.ReplyToMessage.From.UserName == bot.Self.UserName {
				log.Printf("[%s] Replied to us, asking: '%s'", update.Message.From.UserName, update.Message.Text)
				answerRequired = true
			}

			// check if we're being mentioned, answer back if necessary.
			if strings.ToLower(tokens[0]) == strings.ToLower(botUsername) {
				log.Printf("[%s] Mentioned us, asking: '%s'", update.Message.From.UserName, update.Message.Text)
				// pop the first element
				tokens = tokens[1:]
				answerRequired = true
			}

			if answerRequired {
				if dice := rollDice(); dice > 4 {
					gifpick := fmt.Sprintf("%s/%s", gifdb.Store, gifdb.GetRandom())
					log.Printf("[%s] Sending GIF: %s", update.Message.From.UserName, gifpick)
					msg := tgbotapi.NewDocumentUpload(update.Message.Chat.ID, gifpick)
					msg.ReplyToMessageID = update.Message.MessageID
					bot.Send(msg)
				} else {
					genText, elapsed = markov.GenerateChain(numw, update.Message.Text)
					log.Printf("[%s] Sending response: '%s' (generated in %s)", update.Message.From.UserName, genText, elapsed.String())
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, genText)
					msg.ReplyToMessageID = update.Message.MessageID
					bot.Send(msg)
				}
			}

			// add message to chain
			markov.AddChain(strings.Join(tokens, " "))
			if update.Message.Document != nil && (update.Message.Document.MimeType == "video/mp4" && update.Message.Document.FileSize < gifmaxsize) {
				if err := gifdb.Hoard(update, bot); err != nil {
					log.Printf("[%s] Could not save GIF due to error '%s'", update.Message.From.UserName, err)
					return
				}
				gifdb.Add(fmt.Sprintf("%s.mp4", update.Message.Document.FileID))
			}
		}()
	}
}

func rollDice() int {
	dice := []int{1, 2, 3, 4, 5, 6}
	rand.Seed(time.Now().UnixNano())
	return dice[rand.Intn(len(dice)-1)]
}
