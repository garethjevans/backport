package webhook_test

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	"github.com/garethjevans/backport/pkg/webhook"
	http2 "github.com/stretchr/testify/http"

	"github.com/jenkins-x/go-scm/scm"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type WebhookTestSuite struct {
	suite.Suite
	SCMClient  scm.Client
	Controller *webhook.Controller
	TestRepo   scm.Repository
}

func (suite *WebhookTestSuite) TestProcessWebhookPRComment() {
	t := suite.T()
	w := &scm.PullRequestCommentHook{
		Action: scm.ActionUpdate,
		Repo:   suite.TestRepo,
	}

	l := logrus.WithField("test", t.Name())
	entry, message, err := suite.Controller.ProcessWebHook(l, w)

	assert.NoError(t, err)
	assert.Equal(t, "processed PR comment hook", message)
	assert.NotNil(t, entry)
}

func (suite *WebhookTestSuite) TestProcessWebhookPR() {
	t := suite.T()

	w := &scm.PullRequestHook{
		Action: scm.ActionCreate,
		Repo:   suite.TestRepo,
	}

	l := logrus.WithField("test", t.Name())
	entry, message, err := suite.Controller.ProcessWebHook(l, w)

	assert.NoError(t, err)
	assert.Equal(t, "processed PR hook", message)
	assert.NotNil(t, entry)
}

func (suite *WebhookTestSuite) TestProcessWebhookPRReview() {
	t := suite.T()

	w := &scm.ReviewHook{
		Action: scm.ActionSubmitted,
		Repo:   suite.TestRepo,
		Review: scm.Review{
			State: "APPROVED",
			Author: scm.User{
				Login: "user",
				Name:  "User",
			},
		},
	}

	l := logrus.WithField("test", t.Name())
	entry, message, err := suite.Controller.ProcessWebHook(l, w)

	assert.NoError(t, err)
	assert.Equal(t, "ignored webhook review", message)
	assert.NotNil(t, entry)
}

func (suite *WebhookTestSuite) TestParseWebHook() {
	t := suite.T()

	pingBytes, err := os.ReadFile("testdata/ping.json")
	assert.NoError(t, err)

	w := &http2.TestResponseWriter{}

	r, err := http.NewRequest("POST", "/", bytes.NewReader(pingBytes))
	assert.NoError(t, err)

	r.Header.Add("X-GitHub-Delivery", "27579b2c-c262-11ed-90c1-3124ac07309e")
	r.Header.Add("X-GitHub-Event", "push")

	suite.Controller.DefaultHandler(w, r)

	assert.Equal(t, http.StatusOK, w.StatusCode)
}

func (suite *WebhookTestSuite) SetupSuite() {
	suite.Controller = &webhook.Controller{}
	suite.TestRepo = scm.Repository{
		ID:        "1",
		Namespace: "default",
		Name:      "test-repo",
		FullName:  "test-org/test-repo",
		Branch:    "master",
		Private:   false,
	}
}

func TestWebhookTestSuite(t *testing.T) {
	assert.NoError(t, os.Setenv("GIT_TOKEN", "abc123"))
	suite.Run(t, new(WebhookTestSuite))
}
