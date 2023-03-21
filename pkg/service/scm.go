package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/sirupsen/logrus"
)

const (
	black       = "000000"
	labelPrefix = "Backport to "
)

type Scm interface {
	ListCommitsForPr(owner string, repo string, pr int) ([]string, error)
	DetermineBranchesForPr(owner string, repo string, pr int) ([]string, error)
	ApplyCommitsToRepo(owner string, repo string, pr int, branch string, commits []string) error
	AddBranchLabelToPr(owner string, repo string, pr int, branch string) error
}

type scmImpl struct {
	client *scm.Client
	host   string
}

func NewScm(host string, token string) Scm {
	c, err := factory.NewClient("github", host, token)
	if err != nil {
		panic(err)
	}
	return &scmImpl{
		client: c,
		host:   host,
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
		if strings.HasPrefix(label.Name, labelPrefix) {
			branches = append(branches, strings.TrimPrefix(label.Name, labelPrefix))
		}
	}

	return branches, nil
}

func (s *scmImpl) ApplyCommitsToRepo(owner string, repo string, pr int, branch string, commits []string) error {
	logrus.Infof("Applying commits to repo for %s/%s/pulls/%d", owner, repo, pr)
	// clone repository to a temporary directory
	file, err := os.CreateTemp("", "git-worker")
	if err != nil {
		logrus.Fatalf("unable to create temp dir %v", err)
	}
	defer os.Remove(file.Name())

	logrus.Infof("running in directory %s", file.Name())

	gitURL := fmt.Sprintf("%s/%s/%s", s.host, owner, repo)
	o, err := executeGit(file.Name(), "clone", gitURL)
	if err != nil {
		return err
	}
	logrus.Infof("clone> %s", o)

	path := filepath.Join(file.Name(), repo)

	o, err = executeGit(path, "checkout", "", branch)
	if err != nil {
		return err
	}
	logrus.Infof("checkout> %s", o)

	backportBranchName := fmt.Sprintf("backport-PR-%d-to-%s", pr, branch)
	o, err = executeGit(path, "checkout", "-b", backportBranchName)
	if err != nil {
		return err
	}
	logrus.Infof("checkout -b> %s", o)

	// apply commits in order
	for _, commit := range commits {
		logrus.Infof("cherry-picking %s", commit)
		o, err = executeGit(path, "cherry-pick", commit)
		if err != nil {
			return err
		}
		logrus.Infof("cherry-pick> %s", o)
	}

	logrus.Infof("pushing %s", backportBranchName)
	o, err = executeGit(path, "push", "origin", backportBranchName)
	if err != nil {
		return err
	}
	logrus.Infof("push> %s", o)

	// if this fails at any point, create an issue on the repo with labels and the error message

	return nil
}

func (s *scmImpl) AddBranchLabelToPr(owner string, repo string, pr int, branch string) error {
	labelName := fmt.Sprintf("%s%s", labelPrefix, branch)
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
		path := fmt.Sprintf("repos/%s/labels", fmt.Sprintf("%s/%s", owner, repo))
		data, err := json.Marshal(label{
			name:        labelName,
			description: fmt.Sprintf("When this PR is merged, backport this to %s", branch),
			color:       black,
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

type label struct {
	name        string
	description string
	color       string
}

func executeGit(dir string, args ...string) (string, error) {
	logrus.Infof("Running git %s in dir %s", strings.Join(args, " "), dir)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	stdout, err := cmd.CombinedOutput()
	return string(stdout), err
}
