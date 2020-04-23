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
	bolt "go.etcd.io/bbolt"
)

const (
	layout         = "15:04:05.000"
	defaultMessage = "I AM NOCINO"
)

type Prefix []string

func (p Prefix) String() string {
	return strings.Join(p, " ")
}

func (p Prefix) Shift(word string) {
	copy(p, p[1:])
	p[len(p)-1] = word
}

type Chain struct {
	prefixLen int
	mutex     sync.Mutex
	log       *logrus.Entry
	DB        *bolt.DB
}

type oldChain struct {
	Chain map[string][]string
}

// NewChain initializes a new Chain struct.
func NewChain(prefixLen int, logger *logrus.Logger) *Chain {
	logfield := logger.WithField("component", "markov")
	return &Chain{
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

		c.log.Debugf("reading key '%s' from database", key)
		v, err := c.readDB([]byte(key))
		if err != nil {
			c.log.Errorf("error when reading from DB: '%s'", err)
		}

		// tossSalad takes the data, dedupes and add the new word to it
		c.log.Debugf("tossing salad with salad length %d and ingredient '%s'", len(v), s)
		buf, err := c.tossSalad(v, s)
		if err != nil {
			c.log.Errorf("error when tossing salad: '%s'", err)
		}

		c.log.Debugf("writing key '%s' to database with payload length: %d", key, len(buf))
		err = c.writeDB([]byte(key), buf)
		if err != nil {
			c.log.Errorf("error when writing to DB: '%s'", err)
		}

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
	var words []string
	c.log.Debugf("Candidates found: %d", len(candidates))
	if len(candidates) > 0 {
		for _, i := range rand.Perm(len(candidates)) {
			var found []byte
			v := candidates[i]
			c.log.Debugf("Evaluating word: %q", v)
			evalW := fmt.Sprintf(" %s", v)
			found, err := c.readDB([]byte(evalW))
			if err != nil {
				c.log.Errorf("error when reading from DB: '%s'", err)
			}
			if found != nil {
				c.log.Debugf("Found starting word to use for chain: %q", v)
				words = append(words, v)
				p.Shift(v)
				break
			}
		}
	}
	for i := 0; i < n; i++ {
		var choices []string
		c.log.Debugf("generating markov chain: reading '%s' from DB", p.String())
		v, err := c.readDB([]byte(p.String()))
		if err != nil {
			c.log.Errorf("error when reading from DB: '%s'", err)
		}

		json.Unmarshal(v, &choices)

		if len(choices) == 0 {
			c.log.Debugf("we ran out of choices, breaking out of markov chain generation")
			break
		}

		next := choices[rand.Intn(len(choices))]
		words = append(words, next)
		c.log.Debugf("generating markov chain: words connected '%v'", words)
		p.Shift(next)
	}
	return strings.Join(words, " "), time.Since(t)
}

// ReadState reads from a json-formatted state file.
func (c *Chain) ReadState(fileName string) {
	_, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		c.log.Warnf("State file %s not present, creating a new one", fileName)
		oldStateFile := fmt.Sprintf("%s.gz", strings.TrimSuffix(fileName, ".db"))
		c.log.Warnf("Verifying if old state file exists (guessing: %s)", oldStateFile)
		if _, err := os.Stat(oldStateFile); err == nil {
			c.log.Warnf("Old state file exists, importing chain to new format")
			c.ImportOldState(oldStateFile, fileName)
		}
	}
	bdb, err := bolt.Open(fileName, 0600, &bolt.Options{Timeout: 1 * time.Second})
	c.DB = bdb

	var bucketStats int
	err = c.DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("Chain"))
		if err != nil {
			return err
		}
		bucketStats = b.Stats().KeyN
		return nil
	})
	if err != nil {
		c.log.Errorf("boltdb transaction failed with: '%s'", err)
	}
	c.log.Infof("Loaded state from '%s' (%d suffixes).", fileName, bucketStats)
	return
}

// ImportOldState imports old state from GZIP'd state file
func (c *Chain) ImportOldState(oldStateFile string, newFilename string) {
	oldState, err := os.Open(oldStateFile)
	if err != nil {
		c.log.Errorf("old state file '%s' was there a moment ago...", oldStateFile)
	}
	defer oldState.Close()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	gzstream, err := gzip.NewReader(oldState)
	if err != nil {
		c.log.Warnf("Cannot open GZ stream on file %s, skipping import", oldState.Name())
		return
	}
	defer gzstream.Close()
	oldc := &oldChain{
		Chain: make(map[string][]string),
	}
	dec := json.NewDecoder(gzstream)
	dec.Decode(oldc)

	// Open/Create new database
	c.log.Infof("Creating new state file at '%s'.", newFilename)
	idb, err := bolt.Open(newFilename, 0600, &bolt.Options{Timeout: 1 * time.Second})
	defer idb.Close()

	c.log.Infof("importing previous state from '%s' (%d suffixes) to '%s'.", oldState.Name(), len(oldc.Chain), newFilename)
	err = idb.Batch(func(tx *bolt.Tx) error {
		c.log.Debugf("creating boltdb bucket: '%s'", "Chain")
		b, err := tx.CreateBucket([]byte("Chain"))
		if err != nil {
			c.log.Errorf("error when creating new bucket in state: %s", err)
			return err
		}
		for k, v := range oldc.Chain {
			// k is property, v is slice of words
			// deduplicate v on import
			ddv := Deduplicate(v)
			if len(ddv) < len(v) {
				c.log.Debugf("deduplicated slice '%s' from %d elements to %d elements", k, len(v), len(ddv))
			}
			buf, err := json.Marshal(ddv)
			if err != nil {
				c.log.Errorf("error when marshaling %+v to bytes", ddv)
				return err
			}
			b.Put([]byte(k), buf)
		}
		return nil
	})
	if err != nil {
		c.log.Errorf("boltdb transaction failed with: '%s'", err)
	}

	return
}

func (c *Chain) readDB(key []byte) ([]byte, error) {
	var value []byte
	err := c.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Chain"))
		value = b.Get(key)
		return nil
	})
	return value, err
}

func (c *Chain) writeDB(key, value []byte) error {
	return c.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Chain"))
		err := b.Put(key, value)
		return err
	})
}

func (c *Chain) tossSalad(salad []byte, ingredient string) ([]byte, error) {
	var wordSalad []string
	var found bool
	if salad != nil {
		// if salad is not nil, we unmarshal it and look into it to see if we have to write
		err := json.Unmarshal(salad, &wordSalad)
		if err != nil {
			c.log.Errorf("error when unmarshaling %+v to json, len '%d'", salad, len(salad))
			return nil, err
		}
		c.log.Debugf("fetched wordSalad with length: %d", len(wordSalad))
		for _, v := range wordSalad {
			if ingredient == v {
				// we set found = true, and let it skip
				found = true
				break
			}
		}
	} else {
		// if salad is nil, we append the first item to it and return
		wordSalad = append(wordSalad, ingredient)
		c.log.Debugf("empty wordSalad, appending first item to it")
		return json.Marshal(wordSalad)
	}

	if !found {
		// if it wasn't found in the search, we just append to it
		wordSalad = append(wordSalad, ingredient)
		c.log.Debugf("appending this ingredient to wordSalad: '%s'", ingredient)
	}

	return json.Marshal(wordSalad)
}

// Deduplicate returns a new slice with duplicates values removed.
func Deduplicate(s []string) []string {
	if len(s) <= 1 {
		return s
	}

	result := []string{}
	seen := make(map[string]struct{})
	for _, val := range s {
		if _, ok := seen[val]; !ok {
			result = append(result, val)
			seen[val] = struct{}{}
		}
	}
	return result
}
