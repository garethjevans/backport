package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Service interface {
	GetCredentials(host string) (string, string, error)
}

type serviceImpl struct {
	client *scm.Client
}

func NewService() Service {
	return &serviceImpl{}
}

func (s *serviceImpl) GetCredentials(host string) (string, string, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return "", "", err
	}

	const (
		namespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	)

	namespace, err := os.ReadFile(namespaceFile)
	if err != nil {
		return "", "", err
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", "", err
	}

	// use the pods service account to list all secrets
	secrets, err := clientset.CoreV1().Secrets(string(namespace)).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", "", err
	}

	logrus.Infof("There are %d secrets to check", len(secrets.Items))

	for _, secret := range secrets.Items {
		// and type: kubernetes.io/basic-auth
		if secret.Type == "kubernetes.io/basic-auth" {
			for k, v := range secret.Annotations {
				// locate secret with the annotation tekton.dev/git-0: https://github.com
				if strings.HasPrefix(k, "tekton.dev/git-") && v == host {
					// when we find one, we should return data.username and data.password
					return string(secret.Data["username"]), string(secret.Data["password"]), nil
				}
			}
		}
	}

	return "", "", nil
}

func (s *serviceImpl) ListCommitsForPr(owner string, repo string, pr int) ([]string, error) {
	// convert these into commits
	_, _, err := s.client.PullRequests.ListCommits(context.Background(), fmt.Sprintf("%s/%s", owner, repo), pr, &scm.ListOptions{})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *serviceImpl) DetermineBranchesForPr(owner string, repo string, pr int) ([]string, error) {
	// convert these into commits
	_, _, err := s.client.PullRequests.ListCommits(context.Background(), fmt.Sprintf("%s/%s", owner, repo), pr, &scm.ListOptions{})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *serviceImpl) ApplyCommitsToRepo(owner string, repo string, branch string) error {
	// clone repository to a temporary directory

	// checkout branch

	// apply commits in order

	// push local branch to remote

	// create a pull request with the appropriate labels

	// if this fails at any point, create an issue on the repo with labels and the error message

	return nil
}
