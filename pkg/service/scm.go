package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/sirupsen/logrus"
)

const (
	LabelPrefix = "Backport to "
	black       = "000000"
)

type Scm interface {
	ListCommitsForPr(owner string, repo string, pr int) ([]string, error)
	DetermineBranchesForPr(owner string, repo string, pr int) ([]string, error)
	ApplyCommitsToRepo(owner string, repo string, pr int, branch string, commits []string) error
	ListBranchesForRepo(owner string, repo string) ([]string, error)
	AddCommentToPr(owner string, repo string, pr int, comment string) error
	AddLabelToPr(owner string, repo string, pr int, label string) error
}

type scmImpl struct {
	client   *scm.Client
	host     string
	username string
	token    string
}

func NewScm(host string, username string, token string) Scm {
	c, err := factory.NewClient("github", host, token)
	if err != nil {
		panic(err)
	}
	return &scmImpl{
		client:   c,
		host:     host,
		username: username,
		token:    token,
	}
}

func (s *scmImpl) ListCommitsForPr(owner string, repo string, pr int) ([]string, error) {
	// convert these into commits
	commits, _, err := s.client.PullRequests.ListCommits(context.Background(), fmt.Sprintf("%s/%s", owner, repo), pr, &scm.ListOptions{})
	if err != nil {
		return nil, err
	}

	var c []string
	for _, commit := range commits {
		c = append(c, commit.Sha)
	}

	logrus.Infof("got commits %s", c)
	return c, nil
}

func (s *scmImpl) DetermineBranchesForPr(owner string, repo string, pr int) ([]string, error) {
	logrus.Infof("Determining branches for %s/%s/pulls/%d", owner, repo, pr)
	// convert these into commits
	pullRequest, _, err := s.client.PullRequests.Find(context.Background(), fmt.Sprintf("%s/%s", owner, repo), pr)
	if err != nil {
		return nil, err
	}

	var branches []string
	for _, label := range pullRequest.Labels {
		if strings.HasPrefix(label.Name, LabelPrefix) {
			branches = append(branches, strings.TrimPrefix(label.Name, LabelPrefix))
		}
	}

	return branches, nil
}

func (s *scmImpl) ApplyCommitsToRepo(owner string, repo string, pr int, branch string, commits []string) error {
	gitter := NewGitter()

	logrus.Infof("Applying commits to repo for %s/%s/pulls/%d", owner, repo, pr)
	// clone repository to a temporary directory
	file, err := os.MkdirTemp("", "git-worker")
	if err != nil {
		logrus.Fatalf("unable to create temp dir %v", err)
	}
	defer os.RemoveAll(file)

	logrus.Infof("running in directory %s", file)

	gitURL := fmt.Sprintf("%s/%s/%s", s.host, owner, repo)
	_, err = gitter.ExecuteGit(file, "clone", gitURL)
	if err != nil {
		gitter.Messages = append(gitter.Messages, "```")
		_ = s.AddCommentToPr(owner, repo, pr, strings.Join(gitter.Messages, "\n"))
		return err
	}

	path := filepath.Join(file, repo)

	_, err = gitter.ExecuteGit(path, "checkout", branch)
	if err != nil {
		gitter.Messages = append(gitter.Messages, "```")
		_ = s.AddCommentToPr(owner, repo, pr, strings.Join(gitter.Messages, "\n"))
		return err
	}

	// determine a unique branch name
	backportBranchName := fmt.Sprintf("backport-PR-%d-to-%s", pr, branch)
	_, err = gitter.ExecuteGit(path, "checkout", "-b", backportBranchName)
	if err != nil {
		gitter.Messages = append(gitter.Messages, "```")
		_ = s.AddCommentToPr(owner, repo, pr, strings.Join(gitter.Messages, "\n"))
		return err
	}

	_, err = gitter.ExecuteGit(path, "config", "user.email", fmt.Sprintf("%s@users.noreply.github.com", s.username))
	if err != nil {
		gitter.Messages = append(gitter.Messages, "```")
		_ = s.AddCommentToPr(owner, repo, pr, strings.Join(gitter.Messages, "\n"))
		return err
	}

	_, err = gitter.ExecuteGit(path, "config", "user.name", s.username)
	if err != nil {
		gitter.Messages = append(gitter.Messages, "```")
		_ = s.AddCommentToPr(owner, repo, pr, strings.Join(gitter.Messages, "\n"))
		return err
	}

	// apply commits in order
	for _, commit := range commits {
		logrus.Infof("cherry-picking %s", commit)
		_, err = gitter.ExecuteGit(path, "cherry-pick", commit)
		if err != nil {
			gitter.Messages = append(gitter.Messages, "```")
			_ = s.AddCommentToPr(owner, repo, pr, strings.Join(gitter.Messages, "\n"))
			return err
		}
	}

	// don't use the gitter to avoid logging
	_, err = executeGit(path, "config", fmt.Sprintf("url.https://%s:%s@github.com.insteadOf", s.username, s.token), "https://github.com")
	if err != nil {
		gitter.Messages = append(gitter.Messages, "```")
		_ = s.AddCommentToPr(owner, repo, pr, strings.Join(gitter.Messages, "\n"))
		return err
	}

	logrus.Infof("pushing %s", backportBranchName)
	_, err = gitter.ExecuteGit(path, "push", "origin", backportBranchName)
	if err != nil {
		gitter.Messages = append(gitter.Messages, "```")
		_ = s.AddCommentToPr(owner, repo, pr, strings.Join(gitter.Messages, "\n"))
		return err
	}

	logrus.Infof("creating PR")
	prInput := scm.PullRequestInput{
		Title: fmt.Sprintf("Backporting PR-%d to %s", pr, branch),
		Head:  backportBranchName,
		Base:  branch,
		Body:  fmt.Sprintf("Backport from %s/%s/%s/pulls/%d", s.host, owner, repo, pr),
	}

	pullRequest, _, err := s.client.PullRequests.Create(context.Background(), fmt.Sprintf("%s/%s", owner, repo), &prInput)
	if err != nil {
		gitter.Messages = append(gitter.Messages, "```")
		_ = s.AddCommentToPr(owner, repo, pr, strings.Join(gitter.Messages, "\n"))
		return err
	}

	gitter.Messages = append(gitter.Messages, "```")
	gitter.Messages = append(gitter.Messages, fmt.Sprintf("Created PR %s/%s/%s/pulls/%d", s.host, owner, repo, pullRequest.Number))

	// if this fails at any point, create an issue on the repo with labels and the error message

	return s.AddCommentToPr(owner, repo, pr, strings.Join(gitter.Messages, "\n"))
}

func (s *scmImpl) AddCommentToPr(owner string, repo string, pr int, comment string) error {
	_, _, err := s.client.PullRequests.CreateComment(context.Background(), fmt.Sprintf("%s/%s", owner, repo), pr, &scm.CommentInput{
		Body: comment,
	})
	return err
}

func (s *scmImpl) AddLabelToPr(owner string, repo string, pr int, labelName string) error {
	logrus.Infof("Applying label %s to repo for %s/%s/pulls/%d", labelName, owner, repo, pr)

	labels, _, err := s.client.Repositories.ListLabels(context.Background(), fmt.Sprintf("%s/%s", owner, repo), &scm.ListOptions{})
	if err != nil {
		return err
	}

	exists := false
	for _, label := range labels {
		if label.Name == labelName {
			exists = true
		}
	}

	if !exists {
		path := fmt.Sprintf("repos/%s/%s/labels", owner, repo)
		data, err := json.Marshal(label{
			name:  labelName,
			color: black,
		})
		if err != nil {
			return err
		}
		req := &scm.Request{Method: "POST", Path: path, Body: bytes.NewReader(data)}
		_, err = s.client.Do(context.Background(), req)
		if err != nil {
			return err
		}
	}

	// convert these into commits
	_, err = s.client.PullRequests.AddLabel(context.Background(), fmt.Sprintf("%s/%s", owner, repo), pr, labelName)
	if err != nil {
		return err
	}
	return nil
}

func (s *scmImpl) ListBranchesForRepo(owner string, repo string) ([]string, error) {
	var branchesToReturn []string
	path := fmt.Sprintf("repos/%s/%s/branches", owner, repo)
	req := &scm.Request{Method: "GET", Path: path, Body: nil}
	resp, err := s.client.Do(context.Background(), req)
	if err != nil {
		return branchesToReturn, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return branchesToReturn, err
	}

	var branches []branch
	err = json.Unmarshal(body, &branches)
	if err != nil {
		return branchesToReturn, err
	}

	for _, branch := range branches {
		branchesToReturn = append(branchesToReturn, branch.name)
	}

	return branchesToReturn, nil
}

type label struct {
	name  string
	color string
}

type branch struct {
	name string
}

func executeGit(dir string, args ...string) (string, error) {
	logrus.Infof("> git %s in dir %s", strings.Join(args, " "), dir)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	stdout, err := cmd.CombinedOutput()
	logrus.Infof("< %s", stdout)
	return string(stdout), err
}

func NewGitter() observableGitter {
	return observableGitter{
		Messages: []string{"```"},
	}
}

type observableGitter struct {
	Messages []string
}

func (o *observableGitter) ExecuteGit(dir string, args ...string) (string, error) {
	o.Messages = append(o.Messages, fmt.Sprintf("git %s", strings.Join(args, " ")))
	output, err := executeGit(dir, args...)
	o.Messages = append(o.Messages, output)

	return output, err
}
