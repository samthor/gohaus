package main

import (
	"log"
	"strings"
)

func encodeTopic(t string) (out string) {
	if strings.ContainsRune(t, '-') {
		log.Fatalf("can't encode -")
	}
	return strings.ReplaceAll(t, "/", "-")
}
