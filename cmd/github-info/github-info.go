package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

const pageLimit = -1

func main() {
	log.SetFlags(0)
	log.SetPrefix("github_info: ")
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

type stats struct {
	companyStats map[string]*company
	userStats    map[string]*user
}

type user struct {
	name                  string
	company               *company
	issues, prs, comments int
	star                  bool
}

type company struct {
	name                                string
	users, issues, prs, comments, stars int
}

func newStats() *stats {
	return &stats{
		companyStats: make(map[string]*company),
		userStats:    make(map[string]*user),
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("github_info", flag.ContinueOnError)
	var tokenFile, orgName, repoName string
	fs.StringVar(&tokenFile, "token_file", "", "file containing GitHub OAuth2 token")
	fs.StringVar(&orgName, "org", "", "organization name")
	fs.StringVar(&repoName, "repo", "", "repo name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if tokenFile == "" {
		return fmt.Errorf("-token_file not set")
	}
	if orgName == "" {
		return fmt.Errorf("-org not set")
	}
	if repoName == "" {
		return fmt.Errorf("-repo not set")
	}
	token, err := ioutil.ReadFile(tokenFile)
	if err != nil {
		return err
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: string(token)})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	client.UserAgent = "jayconrod github_info"

	stats := newStats()

	if err := processIssues(ctx, client, stats, orgName, repoName); err != nil {
		return err
	}
	if err := processComments(ctx, client, stats, orgName, repoName); err != nil {
		return err
	}
	if err := processStars(ctx, client, stats, orgName, repoName); err != nil {
		return err
	}

	stats.writeCsv(os.Stdout)
	return nil
}

func (s *stats) haveUser(name string) bool {
	_, ok := s.userStats[name]
	return ok
}

func (s *stats) addUser(u *github.User) {
	if s.haveUser(*u.Login) {
		panic("already have user")
	}
	var comp *company
	if u.Company != nil {
		var ok bool
		comp, ok = s.companyStats[*u.Company]
		if !ok {
			comp = &company{name: *u.Company}
			s.companyStats[*u.Company] = comp
		}
		comp.users++
	}
	s.userStats[*u.Login] = &user{
		name:    *u.Login,
		company: comp,
	}
}

func (s *stats) addIssue(issue *github.Issue) {
	u := s.userStats[*issue.User.Login]
	if issue.IsPullRequest() {
		u.prs++
		if u.company != nil {
			u.company.prs++
		}
	} else {
		u.issues++
		if u.company != nil {
			u.company.issues++
		}
	}
}

func (s *stats) addComment(comment *github.IssueComment) {
	u := s.userStats[*comment.User.Login]
	u.comments++
	if u.company != nil {
		u.company.comments++
	}
}

func (s *stats) addStar(name string) {
	u := s.userStats[name]
	u.star = true
	if u.company != nil {
		u.company.stars++
	}
}

func (s *stats) writeCsv(w io.Writer) {
	fmt.Fprintf(w, "Organizations\nName,Users,Issues,PRs,Comments,Stars\n")
	for _, comp := range s.companyStats {
		fmt.Fprintf(w, "%q,%d,%d,%d,%d,%d\n", comp.name, comp.users, comp.issues, comp.prs, comp.comments, comp.stars)
	}
	fmt.Fprintf(w, "\nUsers\nName,Company,Issues,PRs,Comments,Star\n")
	for _, user := range s.userStats {
		companyName := ""
		if user.company != nil {
			companyName = user.company.name
		}
		fmt.Fprintf(w, "%q,%q,%d,%d,%d,%v\n", user.name, companyName, user.issues, user.prs, user.comments, user.star)
	}
}

func processIssues(ctx context.Context, client *github.Client, stats *stats, orgName, repoName string) error {
	issueCount := 0
	page := 1
	for {
		opts := &github.IssueListByRepoOptions{
			ListOptions: github.ListOptions{Page: page, PerPage: 100},
			State:       "all",
			Sort:        "created",
			Direction:   "desc",
		}
		issues, resp, err := client.Issues.ListByRepo(ctx, orgName, repoName, opts)
		if err != nil {
			return err
		}
		for _, issue := range issues {
			if err := ensureUser(ctx, client, stats, *issue.User.Login); err != nil {
				return err
			}
			stats.addIssue(issue)
			issueCount++
		}
		fmt.Fprintf(os.Stderr, "fetched issues page %d/%d (%d issues; %d users)\n", page, resp.LastPage, issueCount, len(stats.userStats))
		if resp.LastPage == 0 || page == pageLimit {
			break
		}
		page = resp.NextPage
	}
	return nil
}

func processComments(ctx context.Context, client *github.Client, stats *stats, orgName, repoName string) error {
	commentCount := 0
	page := 1
	for {
		opts := &github.IssueListCommentsOptions{
			ListOptions: github.ListOptions{Page: page, PerPage: 100},
			Sort:        "created",
			Direction:   "desc",
		}
		coms, resp, err := client.Issues.ListComments(ctx, orgName, repoName, 0, opts)
		if err != nil {
			return err
		}
		for _, com := range coms {
			if err := ensureUser(ctx, client, stats, *com.User.Login); err != nil {
				return err
			}
			stats.addComment(com)
			commentCount++
		}
		fmt.Fprintf(os.Stderr, "fetched comments page (%d/%d) (%d comments; %d users)\n", page, resp.LastPage, len(coms), len(stats.userStats))
		if resp.LastPage == 0 || page == pageLimit {
			break
		}
		page = resp.NextPage
	}
	return nil
}

func processStars(ctx context.Context, client *github.Client, stats *stats, orgName, repoName string) error {
	starCount := 0
	page := 1
	for {
		opts := &github.ListOptions{
			Page:    page,
			PerPage: 100,
		}
		stars, resp, err := client.Activity.ListStargazers(ctx, orgName, repoName, opts)
		if err != nil {
			return err
		}
		for _, star := range stars {
			if err := ensureUser(ctx, client, stats, *star.User.Login); err != nil {
				return err
			}
			stats.addStar(*star.User.Login)
			starCount++
		}
		fmt.Fprintf(os.Stderr, "fetched stars page %d/%d (%d stars; %d users)\n", page, resp.LastPage, starCount, len(stats.userStats))
		if resp.LastPage == 0 || page == pageLimit {
			break
		}
		page = resp.NextPage
	}
	return nil
}

func ensureUser(ctx context.Context, client *github.Client, stats *stats, name string) error {
	if stats.haveUser(name) {
		return nil
	}
	u, _, err := client.Users.Get(ctx, name)
	if err != nil {
		return err
	}
	stats.addUser(u)
	return nil
}

// want to know
// - list of users with issues, prs, watch, star
// - list of organizations with same
