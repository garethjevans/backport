package service

import (
	"context"
	"fmt"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/sirupsen/logrus"
)

type ScmService interface {
	ListCommitsForPr(owner string, repo string, pr int) ([]string, error)
	DetermineBranchesForPr(owner string, repo string, pr int) ([]string, error)
	ApplyCommitsToRepo(owner string, repo string, branch string) error
}

type scmServiceImpl struct {
	client *scm.Client
}

func NewScmService(host string, token string) ScmService {
	c, err := factory.NewClient("github", host, token)
	if err != nil {
		panic(err)
	}
	return &scmServiceImpl{
		client: c,
	}
}

func (s *scmServiceImpl) ListCommitsForPr(owner string, repo string, pr int) ([]string, error) {
	// convert these into commits
	commits, _, err := s.client.PullRequests.ListCommits(context.Background(), fmt.Sprintf("%s/%s", owner, repo), pr, &scm.ListOptions{})
	if err != nil {
		return nil, err
	}
	logrus.Infof("got commits %+v", commits)
	return nil, nil
}

func (s *scmServiceImpl) DetermineBranchesForPr(owner string, repo string, pr int) ([]string, error) {
	// convert these into commits
	_, _, err := s.client.PullRequests.ListCommits(context.Background(), fmt.Sprintf("%s/%s", owner, repo), pr, &scm.ListOptions{})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *scmServiceImpl) ApplyCommitsToRepo(owner string, repo string, branch string) error {
	// clone repository to a temporary directory

	// checkout branch

	// apply commits in order

	// push local branch to remote

	// create a pull request with the appropriate labels

	// if this fails at any point, create an issue on the repo with labels and the error message

	return nil
}
