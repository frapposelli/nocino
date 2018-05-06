package gif

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/telegram-bot-api.v4"
)

type GIFDB struct {
	List  []string
	Store string
	mutex sync.Mutex
	log   *logrus.Entry
}

func NewGIFDB(gifstore string, logger *logrus.Logger) *GIFDB {
	logfields := logger.WithField("component", "gifdb")
	return &GIFDB{
		Store: gifstore,
		log:   logfields,
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

func (g *GIFDB) ReadList() {
	// Create gifstore if doesn't exist
	if _, err := os.Stat(g.Store); os.IsNotExist(err) {
		g.log.Warnf("Directory '%s' does not exist, creating...", g.Store)
		if err := os.Mkdir(g.Store, 0700); err != nil {
			g.log.Fatalf("Cannot create directory '%s', exiting", g.Store)
		}
	}

	gifs, err := ioutil.ReadDir(g.Store)
	if err != nil {
		return
	}

	for _, file := range gifs {
		g.List = append(g.List, file.Name())
	}
	g.log.Infof("Loaded GIF list from '%s' (%d element/s).", g.Store, len(g.List))
}

func (g *GIFDB) Hoard(update tgbotapi.Update, bot *tgbotapi.BotAPI) error {
	g.log.WithFields(logrus.Fields{
		"username": update.Message.From.UserName,
	}).Infof("Hoarding GIF ID '%s'", update.Message.Document.FileID)
	file, err := bot.GetFileDirectURL(update.Message.Document.FileID)
	if err != nil {
		return err
	}
	out, err := os.Create(fmt.Sprintf("%s/%s.mp4", g.Store, update.Message.Document.FileID))
	if err != nil {
		g.log.Errorf("Can't open file '%s' for writing", fmt.Sprintf("%s/%s.mp4", g.Store, update.Message.Document.FileID))
		return err
	}
	defer out.Close()
	resp, err := http.Get(file)
	if err != nil {
		g.log.Errorf("Can't fetch file '%s' from telegram", file)
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
