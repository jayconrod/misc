package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/tools/txtar"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("fixtestsum", flag.ExitOnError)
	var modName string
	fs.StringVar(&modName, "modname", "go.mod", "name of go.mod file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	for _, arg := range args {
		if err := fixTest(arg, modName); err != nil {
			return err
		}
	}
	return nil
}

func fixTest(testPath, modName string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("fixing test %s: %w", testPath, err)
		}
	}()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	arc, err := txtar.ParseFile(testPath)
	if err != nil {
		return err
	}

	sumName := "go.sum"
	if strings.Contains(modName, "/") {
		sumName = path.Join(path.Dir(modName), "go.sum")
	}
	sumIndex := -1
	modIndex := -1
	for i, f := range arc.Files {
		if f.Name == sumName {
			sumIndex = i
		} else if f.Name == modName {
			modIndex = i
		}
		outPath := filepath.Join(dir, f.Name)
		if strings.Contains(f.Name, "/") {
			if err := os.MkdirAll(filepath.Dir(outPath), 0777); err != nil {
				return err
			}
		}
		if err := ioutil.WriteFile(outPath, f.Data, 0666); err != nil {
			return err
		}
	}
	if modIndex < 0 {
		return fmt.Errorf("go.mod file not found")
	}

	modRootDir := filepath.Dir(filepath.Join(dir, modName))
	cmd := exec.Command("go", "list", "-mod=mod", "all")
	cmd.Dir = modRootDir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running 'go list -mod=mod all': %w", err)
	}

	sumData, err := ioutil.ReadFile(filepath.Join(dir, sumName))
	if err != nil {
		return err
	}
	if sumIndex >= 0 {
		arc.Files[sumIndex].Data = sumData
	} else {
		sumFile := txtar.File{Name: sumName, Data: sumData}
		arc.Files = append(arc.Files[:modIndex+1], append([]txtar.File{sumFile}, arc.Files[modIndex+1:]...)...)
	}
	return ioutil.WriteFile(testPath, txtar.Format(arc), 0666)
}
