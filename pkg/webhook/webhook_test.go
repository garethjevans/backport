package webhook

import (
	"os"
	"testing"

	"github.com/jenkins-x/go-scm/scm"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type WebhookTestSuite struct {
	suite.Suite
	SCMCLient scm.Client
	//GitClient      git.Client
	WebhookOptions *Controller
	TestRepo       scm.Repository
}

func (suite *WebhookTestSuite) TestProcessWebhookPRComment() {
	t := suite.T()
	webhook := &scm.PullRequestCommentHook{
		Action: scm.ActionUpdate,
		Repo:   suite.TestRepo,
	}
	l := logrus.WithField("test", t.Name())
	logrusEntry, message, err := suite.WebhookOptions.ProcessWebHook(l, webhook)
	assert.NoError(t, err)
	assert.Equal(t, "processed PR comment hook", message)
	assert.NotNil(t, logrusEntry)
}

func (suite *WebhookTestSuite) TestProcessWebhookPR() {
	t := suite.T()

	webhook := &scm.PullRequestHook{
		Action: scm.ActionCreate,
		Repo:   suite.TestRepo,
	}
	l := logrus.WithField("test", t.Name())
	logrusEntry, message, err := suite.WebhookOptions.ProcessWebHook(l, webhook)

	assert.NoError(t, err)
	assert.Equal(t, "processed PR hook", message)
	assert.NotNil(t, logrusEntry)
}

func (suite *WebhookTestSuite) TestProcessWebhookPRReview() {
	t := suite.T()

	webhook := &scm.ReviewHook{
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
	logrusEntry, message, err := suite.WebhookOptions.ProcessWebHook(l, webhook)

	assert.NoError(t, err)
	assert.Equal(t, "processed PR review hook", message)
	assert.NotNil(t, logrusEntry)
}

func (suite *WebhookTestSuite) SetupSuite() {
	suite.WebhookOptions = &Controller{}
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
	os.Setenv("GIT_TOKEN", "abc123")
	suite.Run(t, new(WebhookTestSuite))
}
