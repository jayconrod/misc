package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v34/github"
	"golang.org/x/build/gerrit"
	"golang.org/x/oauth2"
)

// TODO: gazelle changes printed twice instead of rules_go changes.

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	var wg sync.WaitGroup
	// Fetch Gerrit changes.
	var gerritCLs []*gerrit.ChangeInfo
	var gerritErr error
	wg.Add(1)
	go func() {
		gerritCLs, gerritErr = fetchGerritChanges(ctx)
		wg.Done()
	}()

	// Fetch GitHub changes.
	wg.Add(2)
	var rulesGoPRs, gazellePRs []*github.Issue
	var rulesGoErr, gazelleErr error
	go func() {
		rulesGoPRs, rulesGoErr = fetchGitHubChanges(ctx, "bazelbuild", "rules_go")
		wg.Done()
	}()
	go func() {
		gazellePRs, gazelleErr = fetchGitHubChanges(ctx, "bazelbuild", "bazel-gazelle")
		wg.Done()
	}()

	wg.Wait()
	for _, err := range []error{gerritErr, rulesGoErr, gazelleErr} {
		if err != nil {
			return err
		}
	}

	// Organize issues.
	type change struct {
		addr, link, status, desc string
	}
	type projectChangeList struct {
		title   string
		changes []change
	}
	projects := []projectChangeList{
		{title: "cmd/go"},
		{title: "Fuzzing"},
		{title: "Documentation"},
		{title: "rules_go"},
		{title: "gazelle"},
	}
	const (
		cmdGoIndex = iota
		fuzzingIndex
		documentationIndex
		rulesGoIndex
		gazelleIndex
	)

	for _, cl := range gerritCLs {
		if cl.Status == "ABANDONED" || strings.Contains(cl.Branch, "release-branch") {
			continue
		}
		var l *[]change
		if strings.Contains(cl.Subject, "[dev.fuzz]") {
			l = &projects[fuzzingIndex].changes
		} else if cl.Project == "website" {
			l = &projects[documentationIndex].changes
		} else {
			l = &projects[cmdGoIndex].changes
		}
		status := ""
		if cl.Status == "NEW" {
			status = "pending"
		} else if cl.Status == "DRAFT" {
			status = "draft"
		}

		*l = append(*l, change{
			addr:   fmt.Sprintf("https://go-review.googlesource.com/c/%s/+/%d", cl.Project, cl.ChangeNumber),
			link:   fmt.Sprintf("%d", cl.ChangeNumber),
			status: status,
			desc:   cl.Subject,
		})
	}

	for _, repoPRs := range []struct {
		l         *[]change
		org, repo string
		prs       []*github.Issue
	}{
		{&projects[rulesGoIndex].changes, "bazelbuild", "rules_go", rulesGoPRs},
		{&projects[gazelleIndex].changes, "bazelbuild", "bazel-gazelle", gazellePRs},
	} {
		for _, pr := range repoPRs.prs {
			status := "" // TODO: use pr.State
			*repoPRs.l = append(*repoPRs.l, change{
				addr:   fmt.Sprintf("https://github.com/%s/%s/pull/%d", repoPRs.org, repoPRs.repo, *pr.Number),
				link:   fmt.Sprintf("#%d", *pr.Number),
				status: status,
				desc:   *pr.Title,
			})
		}
	}

	// Format snippet
	for _, project := range projects {
		fmt.Printf("## %s\n\n", project.title)
		for _, change := range project.changes {
			statusStr := ""
			if change.status != "" {
				statusStr = " (" + change.status + ")"
			}
			fmt.Printf("* [%s](%s)%s - %s\n", change.link, change.addr, statusStr, change.desc)
		}
		fmt.Printf("\n")
	}

	return nil
}

func fetchGerritChanges(ctx context.Context) (changes []*gerrit.ChangeInfo, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("fetching Gerrit changes: %w", err)
		}
	}()
	c := gerrit.NewClient("https://go-review.googlesource.com", gerrit.GitCookiesAuth())
	changes, err = c.QueryChanges(ctx, "owner:jayconrod@google.com -age:10d")
	if err != nil {
		return nil, err
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].ChangeNumber >= changes[i].ChangeNumber })
	return changes, nil
}

func fetchGitHubChanges(ctx context.Context, org, repo string) (changes []*github.Issue, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("fetching GitHub changes: %w", err)
		}
	}()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	token, err := os.ReadFile(filepath.Join(homeDir, ".githubtoken"))
	if err != nil {
		return nil, err
	}
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: string(token)})
	oauth2Client := oauth2.NewClient(ctx, tokenSource)
	c := github.NewClient(oauth2Client)

	fromTime := time.Now().Add(-10 * 24 * time.Hour).Format("2006-01-02")
	query := fmt.Sprintf("is:pr org:%s repo:%s author:jayconrod updated:>=%s", org, repo, fromTime)
	opt := github.SearchOptions{ListOptions: github.ListOptions{PerPage: 100}, Sort: "updated", Order: "asc"}
	result, _, err := c.Search.Issues(ctx, query, &opt)
	if err != nil {
		return nil, err
	}
	return result.Issues, nil
}
