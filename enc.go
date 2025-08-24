package main

import (
	"log"
	"strings"
)

func encodeTopic(t string) (out string) {
	if strings.ContainsRune(t, '_') {
		log.Fatalf("can't encode _")
	}
	return strings.ReplaceAll(t, "/", "_")
}
