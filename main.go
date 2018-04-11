package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/jamiealquiza/envy"
	"gopkg.in/telegram-bot-api.v4"
)

var (
	numw        int
	plen        int
	tgtoken     string
	state       string
	botUsername string
	genText     string
	markov      *Chain
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())

	flag.IntVar(&numw, "numw", 25, "maximum number of words for the markov chain")
	flag.IntVar(&plen, "plen", 2, "chain prefix length")
	flag.StringVar(&state, "state", "nocino.state.gz", "state file for nocino")
	flag.StringVar(&tgtoken, "token", "", "telegram bot token")
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

	// feedback loop
	for update := range updates {
		if update.Message == nil {
			continue
		}

		// tokenize message
		tokens := strings.Split(update.Message.Text, " ")

		if update.Message.Text != "" {
			// If talking to the bot, reply with a chain.
			if strings.ToLower(tokens[0]) == strings.ToLower(botUsername) {
				log.Printf("[%s] Asking: %s", update.Message.From.UserName, update.Message.Text)
				genText = markov.GenerateChain(numw, update.Message.Text)
				log.Printf("[%s] Sending response: %s", update.Message.From.UserName, genText)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, genText)
				msg.ReplyToMessageID = update.Message.MessageID
				bot.Send(msg)
				// pop the first element
				tokens = tokens[1:]
			}

			// add message to chain
			markov.AddChain(strings.Join(tokens, " "))

			// now go and save
			go func() {
				t := time.Now().UTC()
				markov.WriteState(state)
				log.Printf("[DEBUG] state save goroutine ended in %s", time.Since(t).String())
			}()
		}
	}
}
