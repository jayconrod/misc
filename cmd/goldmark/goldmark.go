package main

import (
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

func main() {
	var in io.Reader
	var out io.Writer
	log.SetFlags(0)
	log.SetPrefix("goldmark: ")
	if len(os.Args) < 2 || len(os.Args) > 3 {
		log.Fatalf("usage: goldmark [in [out]]")
	}
	if len(os.Args) >= 2 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		in = f
	}
	if len(os.Args) == 3 {
		f, err := os.Create(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Fatal(err)
			}
		}()
		out = f
	}

	src, err := ioutil.ReadAll(in)
	if err != nil {
		log.Fatal(err)
	}
	md := goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe()))
	if err := md.Convert(src, out); err != nil {
		log.Fatal(err)
	}
}
