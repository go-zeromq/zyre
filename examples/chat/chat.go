// Copyright 2020 The go-zeromq Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/go-zeromq/zyre"
)

func chat(ctx context.Context, input <-chan string, name string) {
	node := zyre.NewZyre(ctx)
	defer node.Stop()

	err := node.Start()
	if err != nil {
		log.Fatalln(err)
	}
	node.Join("CHAT")

	for {
		select {
		case e := <-node.Events():
			fmt.Printf("\r%s%s> ", string(e.Msg), name)
		case msg := <-input:
			node.Shout("CHAT", []byte(msg))
		}
	}
}

func main() {
	input := make(chan string)
	name := flag.String("name", "Zyre", "Your name in the chat session")
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go chat(ctx, input, *name)
	fmt.Printf("%s> ", *name)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input <- fmt.Sprintf("%s: %s\n", *name, scanner.Text())
		fmt.Printf("%s> ", *name)
	}
	if err := scanner.Err(); err != nil {
		log.Fatalln("reading standard input:", err)
	}
}
