// Mostly taken from https://golang.org/doc/codewalk/markov/

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
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
	Chain     map[string][]string
	prefixLen int
	mutex     sync.Mutex
}

// NewChain initializes a new Chain struct.
func NewChain(prefixLen int) *Chain {
	return &Chain{
		Chain:     make(map[string][]string),
		prefixLen: prefixLen,
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
func (c *Chain) GenerateChain(n int, seed string) string {
	log.Println("[DEBUG] Entering markov algorithm...")
	defer log.Println("[DEBUG] Exiting markov algorithm...")
	c.mutex.Lock()
	defer c.mutex.Unlock()
	p := make(Prefix, c.prefixLen)
	var words []string
	for i := 0; i < n; i++ {
		choices := c.Chain[p.String()]
		if len(choices) == 0 {
			break
		}
		next := choices[rand.Intn(len(choices))]
		log.Printf("[DEBUG] [iteration %d] [prefix: %s] Selecting suffix: %s", i, p.String(), next)
		words = append(words, next)
		p.Shift(next)
	}
	return strings.Join(words, " ")
}

// ReadState reads from a json-formatted state file.
func (c *Chain) ReadState(fileName string) (err error) {
	fin, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer fin.Close()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	dec := json.NewDecoder(fin)
	err = dec.Decode(c)
	return nil
}

// WriteState writes to a json-formatted state file.
func (c *Chain) WriteState(fileName string) (err error) {
	fout, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer fout.Close()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	enc := json.NewEncoder(fout)
	err = enc.Encode(c)
	return nil
}
