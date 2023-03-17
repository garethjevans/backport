package service

import (
	"context"
	"fmt"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/sirupsen/logrus"
	"strings"
)

type Scm interface {
	ListCommitsForPr(owner string, repo string, pr int) ([]string, error)
	DetermineBranchesForPr(owner string, repo string, pr int) ([]string, error)
	ApplyCommitsToRepo(owner string, repo string, branch string) error
}

type scmImpl struct {
	client *scm.Client
}

func NewScm(host string, token string) Scm {
	c, err := factory.NewClient("github", host, token)
	if err != nil {
		panic(err)
	}
	return &scmImpl{
		client: c,
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

func (s *scmImpl) ApplyCommitsToRepo(owner string, repo string, branch string) error {
	// clone repository to a temporary directory

	// checkout branch

	// apply commits in order

	// push local branch to remote

	// create a pull request with the appropriate labels

	// if this fails at any point, create an issue on the repo with labels and the error message

	return nil
}
