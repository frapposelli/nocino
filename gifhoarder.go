package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"

	"gopkg.in/telegram-bot-api.v4"
)

type GIFDB struct {
	List  []string
	Store string
	mutex sync.Mutex
}

func NewGIFDB(gifstore string) *GIFDB {
	return &GIFDB{
		Store: gifstore,
	}
}

func (g *GIFDB) Add(in string) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.List = append(g.List, in)

	return nil
}

func (g *GIFDB) GetRandom() string {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	return g.List[rand.Intn(len(g.List))]

}

func (g *GIFDB) ReadList() error {
	// Create gifstore if doesn't exist
	if _, err := os.Stat(g.Store); os.IsNotExist(err) {
		os.Mkdir(gifstore, 0755)
	}

	gifs, err := ioutil.ReadDir(g.Store)
	if err != nil {
		return err
	}

	for _, file := range gifs {
		g.List = append(g.List, file.Name())
	}

	return nil
}

func (g *GIFDB) Hoard(update tgbotapi.Update, bot *tgbotapi.BotAPI) error {
	log.Printf("[%s] Hoarding GIF ID '%s'", update.Message.From.UserName, update.Message.Document.FileID)
	file, err := bot.GetFileDirectURL(update.Message.Document.FileID)
	if err != nil {
		return err
	}
	out, err := os.Create(fmt.Sprintf("%s/%s.mp4", g.Store, update.Message.Document.FileID))
	defer out.Close()
	resp, err := http.Get(file)
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
