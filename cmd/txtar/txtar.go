package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/jayconrod/misc/cmd/txtar/internal/txtar"
)

func main() {
	log.SetPrefix("txtar: ")
	log.SetFlags(0)
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		return create(args)
	}
	return extract()
}

func extract() error {
	data, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	arc := txtar.Parse(data)

	for _, f := range arc.Files {
		path := filepath.FromSlash(f.Name)
		if dir := filepath.Dir(path); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0777); err != nil {
				return err
			}
		}
		if err := ioutil.WriteFile(path, f.Data, 0666); err != nil {
			return err
		}
	}

	return nil
}

func create(args []string) error {
	for _, arg := range args {
		err := filepath.Walk(arg, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if info.Mode()&os.ModeIrregular != 0 {
				log.Printf("skipping irregular file: %s", path)
				return nil
			}
			fmt.Printf("-- %s --\n", path)
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(os.Stdout, f); err != nil {
				return err
			}
			fmt.Print("\n")
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
