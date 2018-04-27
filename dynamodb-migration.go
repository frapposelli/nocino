package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

type Chain struct {
	Chain map[string][]string
}

type DBChain struct {
	Prefix   string   `json:"prefix"`
	Suffixes []string `json:"suffixes"`
}

func main() {

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1")},
	)

	if err != nil {
		fmt.Println("Error creating session:")
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// Create DynamoDB client
	svc := dynamodb.New(sess)

	items := NewChain()

	items.GetItems()

	for k, v := range items.Chain {
		log.Printf("[DEBUG] key: %s | value: %s\n", k, v)
		conv := DBChain{
			Prefix:   k,
			Suffixes: v,
		}
		av, err := dynamodbattribute.MarshalMap(conv)
		if err != nil {
			fmt.Println("Got error marshalling map:")
			fmt.Println(err.Error())
			os.Exit(1)
		}
		// Create item in table Movies
		input := &dynamodb.PutItemInput{
			Item:      av,
			TableName: aws.String("nocino-chain"),
		}

		_, err = svc.PutItem(input)

		if err != nil {
			fmt.Println("Got error calling PutItem:")
			fmt.Println(err.Error())
			os.Exit(1)
		}
	}

}

// NewChain initializes a new Chain struct.
func NewChain() *Chain {
	return &Chain{
		Chain: make(map[string][]string),
	}
}

func (c *Chain) GetItems() (err error) {
	fin, _ := os.Open("nocino.state.gz")
	defer fin.Close()

	gzstream, _ := gzip.NewReader(fin)
	defer gzstream.Close()

	dec := json.NewDecoder(gzstream)
	err = dec.Decode(c)
	return nil
}
