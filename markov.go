// Mostly taken from https://golang.org/doc/codewalk/markov/

package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
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
func (c *Chain) GenerateChain(n int, seed string) (string, time.Duration) {
	t := time.Now().UTC()
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
		words = append(words, next)
		p.Shift(next)
	}
	return strings.Join(words, " "), time.Since(t)
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

	gzstream, err := gzip.NewReader(fin)
	if err != nil {
		return err
	}
	defer gzstream.Close()

	dec := json.NewDecoder(gzstream)
	err = dec.Decode(c)
	return nil
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
