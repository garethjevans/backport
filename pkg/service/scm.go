package service

import (
	"context"
	"fmt"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"strings"
)

type Scm interface {
	ListCommitsForPr(owner string, repo string, pr int) ([]string, error)
	DetermineBranchesForPr(owner string, repo string, pr int) ([]string, error)
	ApplyCommitsToRepo(owner string, repo string, pr int, branch string, commits []string) error
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
	// convert these into commits
	pullRequest, _, err := s.client.PullRequests.Find(context.Background(), fmt.Sprintf("%s/%s", owner, repo), pr)
	if err != nil {
		return nil, err
	}

	var branches []string
	for _, label := range pullRequest.Labels {
		const labelPrefix = "Backport to "
		if strings.HasPrefix(label.Name, labelPrefix) {
			branches = append(branches, strings.TrimPrefix(label.Name, labelPrefix))
		}
	}

	return branches, nil
}

func (s *scmImpl) ApplyCommitsToRepo(owner string, repo string, pr int, branch string, commits []string) error {
	// clone repository to a temporary directory
	file, err := os.CreateTemp("", "git-worker")
	if err != nil {
		logrus.Fatalf("unable to create temp dir %v", err)
	}
	defer os.Remove(file.Name())

	gitUrl := fmt.Sprintf("%s/%s/%s", s.host, owner, repo)
	o, err := execute("git", "clone", gitUrl)
	if err != nil {
		return err
	}
	logrus.Infof("got %s", o)

	o, err = execute("git", "checkout", "", branch)
	if err != nil {
		return err
	}
	logrus.Infof("got %s", o)

	backportBranchName := fmt.Sprintf("backport-PR-%d-to-%s", pr, branch)
	o, err = execute("git", "checkout", "-b", backportBranchName)
	if err != nil {
		return err
	}
	logrus.Infof("got %s", o)

	// apply commits in order
	for _, commit := range commits {
		logrus.Infof("cherry-picking %s", commit)
		o, err = execute("git", "cherry-pick", commit)
		if err != nil {
			return err
		}
		logrus.Infof("got %s", o)
	}

	logrus.Infof("pushing %s", backportBranchName)
	o, err = execute("git", "push", "origin", backportBranchName)
	if err != nil {
		return err
	}
	logrus.Infof("got %s", o)

	// FIXME if this fails at any point, create an issue on the repo with labels and the error message

	return nil
}

func execute(command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	stdout, err := cmd.Output()

	if err != nil {
		return "", err
	}

	return string(stdout), nil
}
