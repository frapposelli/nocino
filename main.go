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
	numw           int
	plen           int
	tgtoken        string
	state          string
	botUsername    string
	genText        string
	answerRequired bool
	elapsed        time.Duration
	checkpoint     time.Duration
	markov         *Chain
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())

	flag.IntVar(&numw, "numw", 25, "maximum number of words for the markov chain")
	flag.IntVar(&plen, "plen", 2, "chain prefix length")
	flag.StringVar(&state, "state", "nocino.state.gz", "state file for nocino")
	flag.StringVar(&tgtoken, "token", "", "telegram bot token")
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

	// state file save ticker
	log.Printf("Starting state save ticker with %s interval", checkpoint.String())
	ticker := time.NewTicker(checkpoint)
	go func() {
		for tick := range ticker.C {
			if err := markov.WriteState(state); err != nil {
				log.Printf("[state] checkpoint failed: %s (in %s)", err.Error(), time.Since(tick).String())
			} else {
				log.Printf("[state] checkpoint completed, %d suffixes in chain (in %s)", len(markov.Chain), time.Since(tick).String())
			}
		}
	}()

	// feedback loop
	for update := range updates {
		answerRequired = false
		if update.Message == nil || update.Message.Text == "" {
			continue
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
			genText, elapsed = markov.GenerateChain(numw, update.Message.Text)
			log.Printf("[%s] Sending response: '%s' (generated in %s)", update.Message.From.UserName, genText, elapsed.String())
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, genText)
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Send(msg)
		}

		// add message to chain
		go func() {
			markov.AddChain(strings.Join(tokens, " "))
		}()
	}
}
