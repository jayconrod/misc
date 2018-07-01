package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/buildtools/build"
)

func main() {
	log.SetPrefix("add-3p-repo: ")
	log.SetFlags(0)

	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("add-3p-repo", flag.ContinueOnError)
	var workspaceName string
	var repo string
	fs.StringVar(&workspaceName, "workspace_name", "", "name of the workspace")
	fs.StringVar(&repo, "repo", "", "repository to create third_party entry for")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("expected 0 positional args; got %d", len(fs.Args()))
	}
	if workspaceName == "" {
		return fmt.Errorf("-workspace_name was not set")
	}
	if repo == "" {
		return fmt.Errorf("-repo was not set")
	}

	if err := cdToRoot(); err != nil {
		return err
	}

	if err := fetch(repo); err != nil {
		return err
	}

	repoDir, err := findRepoDir(repo)
	if err != nil {
		return err
	}

	buildPaths, err := findBuildFiles(repoDir)
	if err != nil {
		return err
	}

	if err := copyBuildFilesToThirdParty(repo, repoDir, buildPaths); err != nil {
		return err
	}

	if err := updateManifest(workspaceName, repo, repoDir, buildPaths); err != nil {
		return err
	}

	return nil
}

func cdToRoot() error {
	dir, err := filepath.Abs(".")
	if err != nil {
		return nil
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "WORKSPACE")); err == nil {
			break
		}
		if strings.HasSuffix(dir, string(os.PathSeparator)) {
			return fmt.Errorf("could not locate WORKSPACE in any parent directory")
		}
		dir = filepath.Dir(dir)
	}
	return os.Chdir(dir)
}

func fetch(repo string) error {
	cmd := exec.Command("bazel", "fetch", "@"+repo+"//:BUILD.bazel")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
	return nil
}

func findRepoDir(repo string) (string, error) {
	externalPath := strings.Join([]string{"bazel-out", "..", "..", "..", "external", repo}, string(os.PathSeparator))
	cleanPath, err := filepath.EvalSymlinks(externalPath)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(cleanPath); err != nil {
		return "", err
	} else if !st.IsDir() {
		return "", fmt.Errorf("%s: not a directory", externalPath)
	}
	return cleanPath, nil
}

func findBuildFiles(repoDir string) ([]string, error) {
	var files []string
	err := filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Base(path) != "BUILD.bazel" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func copyBuildFilesToThirdParty(repo string, repoDir string, buildPaths []string) error {
	thirdPartyDir := filepath.Join("third_party", repo)
	if err := os.MkdirAll(thirdPartyDir, 0777); err != nil {
		return err
	}
	for _, from := range buildPaths {
		rel, _ := filepath.Rel(repoDir, from)
		to := filepath.Join(thirdPartyDir, rel+".in")
		if err := os.MkdirAll(filepath.Dir(to), 0777); err != nil {
			return err
		}
		content, err := ioutil.ReadFile(from)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(to, content, 0666); err != nil {
			return err
		}
	}
	return nil
}

func updateManifest(workspaceName, repo, repoDir string, buildPaths []string) error {
	manifestPath := filepath.Join("third_party", "manifest.bzl")
	var file *build.File
	var manifestDict *build.DictExpr
	if content, err := ioutil.ReadFile(manifestPath); err != nil {
		manifestDict = &build.DictExpr{
			ForceMultiLine: true,
		}

		file = &build.File{
			Path: manifestPath,
			Stmt: []build.Expr{
				&build.BinaryExpr{
					X:  &build.LiteralExpr{Token: "manifest"},
					Op: "=",
					Y:  manifestDict,
				},
			},
		}
	} else {
		file, err = build.Parse(manifestPath, content)
		if err != nil {
			return err
		}
		if len(file.Stmt) == 0 {
			return fmt.Errorf("%s: manifest is empty", manifestPath)
		}
		manifestStmt, ok := file.Stmt[0].(*build.BinaryExpr)
		if !ok || manifestStmt.Op != "=" {
			return fmt.Errorf("%s: first statement is not assignment", manifestPath)
		}
		if lit, ok := manifestStmt.X.(*build.LiteralExpr); !ok || lit.Token != "manifest" {
			return fmt.Errorf("%s: first statement is not assignment to manifest", manifestPath)
		}
		manifestDict, ok = manifestStmt.Y.(*build.DictExpr)
		if !ok {
			return fmt.Errorf("%s: first statement is not dict assignment", manifestPath)
		}
	}

	entries := make([]build.Expr, 0, len(buildPaths))
	for _, buildPath := range buildPaths {
		value, _ := filepath.Rel(repoDir, buildPath)
		value = filepath.ToSlash(value)
		key := fmt.Sprintf("@%s//third_party:%s/%s.in", workspaceName, repo, value)
		entry := &build.KeyValueExpr{
			Key:   &build.StringExpr{Value: key},
			Value: &build.StringExpr{Value: value},
		}
		entries = append(entries, entry)
	}

	repoEntry := &build.KeyValueExpr{
		Key: &build.StringExpr{Value: repo},
		Value: &build.DictExpr{
			List:           entries,
			ForceMultiLine: true,
		},
	}
	replacedEntry := false
	for i, entry := range manifestDict.List {
		kv := entry.(*build.KeyValueExpr)
		key, ok := kv.Key.(*build.StringExpr)
		if !ok {
			continue
		}
		if key.Value == repo {
			manifestDict.List[i] = repoEntry
			replacedEntry = true
			break
		}
	}
	if !replacedEntry {
		manifestDict.List = append(manifestDict.List, repoEntry)
	}

	stringKey := func(entry build.Expr) string {
		key, ok := entry.(*build.KeyValueExpr).Key.(*build.StringExpr)
		if !ok {
			fmt.Fprintf(os.Stderr, "no string?\n")
			return ""
		}
		return key.Value
	}
	sort.Slice(manifestDict.List, func(i, j int) bool {
		return stringKey(manifestDict.List[i]) < stringKey(manifestDict.List[j])
	})

	content := build.Format(file)
	return ioutil.WriteFile(file.Path, content, 0666)
}
