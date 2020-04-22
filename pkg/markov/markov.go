// Mostly taken from https://golang.org/doc/codewalk/markov/

package markov

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	snowball "github.com/snowballstem/snowball/go"
)

const layout = "15:04:05.000"

type Prefix []string

func (p Prefix) String() string {
	return strings.Join(p, " ")
}

func (p Prefix) Shift(word string) {
	copy(p, p[1:])
	p[len(p)-1] = word
}

type Chain struct {
	Chain     map[string][]string
	prefixLen int
	mutex     sync.Mutex
	log       *logrus.Entry
}

// NewChain initializes a new Chain struct.
func NewChain(prefixLen int, logger *logrus.Logger) *Chain {
	logfield := logger.WithField("component", "markov")
	return &Chain{
		Chain:     make(map[string][]string),
		prefixLen: prefixLen,
		log:       logfield,
	}
}

// AddChain adds a new message to the chain
func (c *Chain) AddChain(in string) (int, error) {
	sr := strings.NewReader(in)
	p := make(Prefix, c.prefixLen)
	for {
		var s string
		if _, err := fmt.Fscan(sr, &s); err != nil {
			break
		}
		key := p.String()
		c.mutex.Lock()
		c.Chain[key] = append(c.Chain[key], s)
		c.mutex.Unlock()
		p.Shift(s)
	}
	return len(in), nil
}

// GenerateChain generates a markov chain.
func (c *Chain) GenerateChain(n int, seed string) (string, time.Duration) {
	t := time.Now().UTC()
	c.mutex.Lock()
	defer c.mutex.Unlock()
	p := make(Prefix, c.prefixLen)
	c.log.Debugf("Stemming and evaluating seed string %q", seed)
	// TODO(frapposelli): remove hardcoded bot name
	seed = strings.TrimPrefix(seed, "@nocino_bot")
	seedSplit := strings.Split(seed, " ")
	var candidates []string
	c.log.Debugf("Evaluating candidates %+v", seedSplit)
	for _, v := range seedSplit {
		if len(v) > 3 {
			candidates = append(candidates, v)
		}
	}
	c.log.Debugf("Candidates found: %d", len(candidates))
	if len(candidates) > 0 {
		for _, v := range candidates {
			c.log.Debugf("Evaluating word: %q", v)
			if _, ok := c.Chain[v]; ok {
				c.log.Debugf("Found starting word to use for chain: %q", v)
				p.Shift(v)
				break
			}
		}
	}
	var words []string
	for i := 0; i < n; i++ {
		choices := c.Chain[p.String()]
		if len(choices) == 0 {
			break
		}
		next := choices[rand.Intn(len(choices))]
		words = append(words, next)
		c.log.Debugf("Generating Markov chain '%v'", words)
		p.Shift(next)
	}
	return strings.Join(words, " "), time.Since(t)
}

// ReadState reads from a json-formatted state file.
func (c *Chain) ReadState(fileName string) {
	fin, err := os.Open(fileName)
	if err != nil {
		c.log.Warnf("State file %s not present, creating a new one", fileName)
		return
	}
	defer fin.Close()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	gzstream, err := gzip.NewReader(fin)
	if err != nil {
		c.log.Warnf("Cannot open GZ stream on file %s, creating a new one", fileName)
		return
	}
	defer gzstream.Close()

	dec := json.NewDecoder(gzstream)
	dec.Decode(c)
	c.log.Infof("Loaded previous state from '%s' (%d suffixes).", fileName, len(c.Chain))

	return
}

// WriteState writes to a json-formatted state file.
func (c *Chain) WriteState(fileName string) (err error) {
	// remember that defers are LIFO
	fout, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer fout.Close()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	gzstream := gzip.NewWriter(fout)
	defer gzstream.Close()

	enc := json.NewEncoder(gzstream)
	err = enc.Encode(c)

	return nil
}

func (c *Chain) RunStateSaveTicker(checkpoint time.Duration, state string) {
	c.log.Infof("Starting state save ticker with %s interval", checkpoint.String())
	ticker := time.NewTicker(checkpoint)
	go func() {
		for tick := range ticker.C {
			if err := c.WriteState(state); err != nil {
				c.log.WithField("elapsed", time.Since(tick).String()).Errorf("checkpoint failed: %s", err.Error())
				return
			}
			c.log.WithField("elapsed", time.Since(tick).String()).Debugf("checkpoint completed, %d suffixes in chain", len(c.Chain))
		}
	}()

}

// stringProcess tokenizes and stems the string
func stemString(in string) []string {
	var returnString []string
	// make the string lowercase
	loweredString := strings.ToLower(in)
	// split the string in array of words
	splitString := strings.Split(loweredString, " ")
	for _, str := range splitString {
		env := snowball.NewEnv(str)
		// Stem the word using snowball
		Stem(env)
		dirty := env.Current()
		var cln string
		// clean the string of any chars that are not a-z A-Z and space
		for _, v := range dirty {
			vb := byte(v)
			switch {
			case vb == 32:
				cln += string(vb)
			case vb >= 65 && vb <= 90:
				cln += string(vb)
			case vb >= 97 && vb <= 122:
				cln += string(vb)
			}
		}

		returnString = append(returnString, cln)
	}
	return returnString
}
