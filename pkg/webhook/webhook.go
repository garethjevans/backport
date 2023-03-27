package webhook

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/garethjevans/backport/pkg/service"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/driver/github"
	"github.com/sirupsen/logrus"
)

// Controller holds the command line arguments.
type Controller struct{}

// Health returns either HTTP 204 if the service is healthy, otherwise nothing ('cos it's dead).
func (o *Controller) Health(w http.ResponseWriter, _ *http.Request) {
	logrus.Debug("Health check")
	w.WriteHeader(http.StatusNoContent)
}

// Ready returns either HTTP 204 if the service is Ready to serve requests, otherwise HTTP 503.
func (o *Controller) Ready(w http.ResponseWriter, _ *http.Request) {
	logrus.Debug("Ready check")
	if o.isReady() {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

// DefaultHandler responds to requests without a specific handler.
func (o *Controller) DefaultHandler(w http.ResponseWriter, r *http.Request) {
	o.HandleWebhookRequests(w, r)
}

func (o *Controller) isReady() bool {
	return true
}

// HandleWebhookRequests handles incoming webhook events.
func (o *Controller) HandleWebhookRequests(w http.ResponseWriter, r *http.Request) {
	o.handleWebhookOrPollRequest(w, r, "Webhook", func(scmClient *scm.Client, r *http.Request) (scm.Webhook, error) {
		return scmClient.Webhooks.Parse(r, o.secretFn)
	})
}

// handleWebhookOrPollRequest handles incoming events.
func (o *Controller) handleWebhookOrPollRequest(w http.ResponseWriter, r *http.Request, operation string, parseWebhook func(scmClient *scm.Client, r *http.Request) (scm.Webhook, error)) {
	if r.Method != http.MethodPost {
		// liveness probe etc
		logrus.WithField("method", r.Method).Debug("invalid http method so returning 200")
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logrus.Errorf("failed to Read Body: %s", err.Error())
		responseHTTPError(w, http.StatusInternalServerError, fmt.Sprintf("500 Internal Server Error: Read Body: %s", err.Error()))
		return
	}

	err = r.Body.Close() // must close
	if err != nil {
		logrus.Errorf("failed to Close Body: %s", err.Error())
		responseHTTPError(w, http.StatusInternalServerError, fmt.Sprintf("500 Internal Server Error: Read Close: %s", err.Error()))
		return
	}

	logrus.Debugf("raw event %s", string(bodyBytes))

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	scmClient := github.NewDefault()

	webhook, err := parseWebhook(scmClient, r)
	if err != nil {
		logrus.Warnf("failed to parse webhook: %s", err.Error())
		responseHTTPError(w, http.StatusBadRequest, fmt.Sprintf("400 Bad Request: Failed to parse webhook: %s", err.Error()))
		return
	}
	if webhook == nil {
		logrus.Error("no webhook was parsed")
		responseHTTPError(w, http.StatusBadRequest, "400 Bad Request: No webhook could be parsed")
		return
	}

	entry := logrus.WithField(operation, webhook.Kind())

	l, output, err := o.ProcessWebHook(entry, webhook)
	if err != nil {
		responseHTTPError(w, http.StatusInternalServerError, fmt.Sprintf("500 Internal Server Error: %s", err.Error()))
	}

	_, err = w.Write([]byte(output))
	if err != nil {
		l.Debugf("failed to process the webhook: %v", err)
	}
}

// ProcessWebHook process a webhook.
func (o *Controller) ProcessWebHook(l *logrus.Entry, webhook scm.Webhook) (*logrus.Entry, string, error) {
	repository := webhook.Repository()
	fields := map[string]interface{}{
		"Repo": fmt.Sprintf("%s/%s", repository.Namespace, repository.Name),
		"Link": repository.Link,
		"Kind": webhook.Kind(),
	}

	l = l.WithFields(fields)

	switch webhook.Kind() {
	case scm.WebhookKindBranch:
		fallthrough
	case scm.WebhookKindCheckRun:
		fallthrough
	case scm.WebhookKindCheckSuite:
		fallthrough
	case scm.WebhookKindDeploy:
		fallthrough
	case scm.WebhookKindDeploymentStatus:
		fallthrough
	case scm.WebhookKindFork:
		fallthrough
	case scm.WebhookKindInstallation:
		fallthrough
	case scm.WebhookKindInstallationRepository:
		fallthrough
	case scm.WebhookKindIssue:
		fallthrough
	case scm.WebhookKindLabel:
		fallthrough
	case scm.WebhookKindPing:
		fallthrough
	case scm.WebhookKindPush:
		fallthrough
	case scm.WebhookKindRelease:
		fallthrough
	case scm.WebhookKindRepository:
		fallthrough
	case scm.WebhookKindReview:
		fallthrough
	case scm.WebhookKindReviewCommentHook:
		fallthrough
	case scm.WebhookKindStar:
		fallthrough
	case scm.WebhookKindStatus:
		fallthrough
	case scm.WebhookKindTag:
		fallthrough
	case scm.WebhookKindWatch:
		return l, fmt.Sprintf("ignored webhook %s", webhook.Kind()), nil
	case scm.WebhookKindPullRequest:
		prHook, ok := webhook.(*scm.PullRequestHook)
		if ok {
			action := prHook.Action
			fields["Action"] = action.String()
			pr := prHook.PullRequest
			fields["PR.Number"] = pr.Number
			fields["PR.Ref"] = pr.Ref
			fields["PR.Sha"] = pr.Sha
			fields["PR.Title"] = pr.Title
			fields["PR.Body"] = pr.Body

			l.Info("invoking PR handler")

			o.handlePullRequestEvent(l, prHook)
			return l, "processed PR hook", nil
		}
	case scm.WebhookKindPullRequestComment:
		prCommentHook, ok := webhook.(*scm.PullRequestCommentHook)
		if ok {
			action := prCommentHook.Action
			fields["Action"] = action.String()
			pr := prCommentHook.PullRequest
			fields["PR.Number"] = pr.Number
			fields["PR.Ref"] = pr.Ref
			fields["PR.Sha"] = pr.Sha
			fields["PR.Title"] = pr.Title
			fields["PR.Body"] = pr.Body
			comment := prCommentHook.Comment
			fields["Comment.Body"] = comment.Body
			author := comment.Author
			fields["Author.Name"] = author.Name
			fields["Author.Login"] = author.Login
			fields["Author.Avatar"] = author.Avatar

			l.Info("invoking PR Comment handler")

			o.handlePullRequestCommentEvent(l, *prCommentHook)
			return l, "processed PR comment hook", nil
		}

	case scm.WebhookKindIssueComment:
		issueCommentHook, ok := webhook.(*scm.IssueCommentHook)
		if ok {
			action := issueCommentHook.Action
			issue := issueCommentHook.Issue
			comment := issueCommentHook.Comment
			sender := issueCommentHook.Sender
			fields["Action"] = action.String()
			fields["Issue.Number"] = issue.Number
			fields["Issue.Title"] = issue.Title
			fields["Issue.Body"] = issue.Body
			fields["Comment.Body"] = comment.Body
			fields["Sender.Body"] = sender.Name
			fields["Sender.Login"] = sender.Login
			fields["Kind"] = "IssueCommentHook"

			l.Info("invoking Issue Comment handler")

			o.handleIssueCommentEvent(l, *issueCommentHook)
			return l, "processed issue comment hook", nil
		}
	}

	l.Debugf("unknown kind %s webhook %#v", webhook.Kind(), webhook)
	return l, fmt.Sprintf("unknown hook %s", webhook.Kind()), nil
}

func (o *Controller) secretFn(scm.Webhook) (string, error) {
	return HMACToken(), nil
}

func (o *Controller) handlePullRequestCommentEvent(l *logrus.Entry, hook scm.PullRequestCommentHook) {
	l.Infof("handling comment on PR-%d", hook.PullRequest.Number)
	l.Infof("new comment '%s'", hook.Comment.Body)

	body := hook.Comment.Body

	parts := strings.Split(hook.Repo.FullName, "/")
	err := o.HandleComment(l, "https://github.com", parts[0], parts[1], body, hook.PullRequest.Number)
	if err != nil {
		logrus.Errorf("Unable to handle PR comment: %v", err)
	}
}

func (o *Controller) handleIssueCommentEvent(l *logrus.Entry, hook scm.IssueCommentHook) {
	l.Infof("handling comment on Issue %d", hook.Issue.Number)
	l.Infof("new comment '%s'", hook.Comment.Body)

	body := hook.Comment.Body

	parts := strings.Split(hook.Repo.FullName, "/")
	err := o.HandleComment(l, "https://github.com", parts[0], parts[1], body, hook.Issue.Number)
	if err != nil {
		logrus.Errorf("Unable to handle issue comment: %v", err)
	}
}

func (o *Controller) HandleComment(l *logrus.Entry, host string, owner string, repo string, body string, pr int) error {
	labels, messages, err := DetermineLabelsToAddFromComment(body, newLabelLister(host, owner, repo))
	if err != nil {
		return err
	}

	for _, label := range labels {
		err := o.addLabelToPr(l, host, owner, repo, pr, label)
		if err != nil {
			return err
		}
	}

	for _, message := range messages {
		err := o.addCommentToPr(l, host, owner, repo, pr, message)
		if err != nil {
			return err
		}
	}

	return nil
}

func newLabelLister(host string, owner string, repo string) Lister {
	return &labelLister{host: host, owner: owner, repo: repo}
}

type labelLister struct {
	host  string
	owner string
	repo  string
}

func (l *labelLister) Branches() ([]string, error) {
	k := service.NewKubernetes()
	u, t, err := k.GetCredentials(l.host)
	if err != nil {
		return nil, err
	}

	s := service.NewScm(l.host, u, t)
	return s.ListBranchesForRepo(l.owner, l.repo)
}

func (o *Controller) applyBackports(l *logrus.Entry, host string, owner string, repo string, pr int) error {
	k := service.NewKubernetes()
	u, t, err := k.GetCredentials(host)
	if err != nil {
		return err
	}

	l.Debugf("username=%s, password=XXX", u)

	s := service.NewScm(host, u, t)
	commits, err := s.ListCommitsForPr(owner, repo, pr)
	if err != nil {
		return err
	}

	l.Infof("commits=%s", commits)

	branches, err := s.DetermineBranchesForPr(owner, repo, pr)
	if err != nil {
		return err
	}

	l.Infof("branches=%s", branches)

	for _, branch := range branches {
		err = s.ApplyCommitsToRepo(owner, repo, pr, branch, commits)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *Controller) handlePullRequestEvent(l *logrus.Entry, hook *scm.PullRequestHook) {
	l.Infof("handling pull request event %d", hook.PullRequest.Number)

	// need to filter on these, we are currently getting to many.
	// only do it on merge?
	if hook.Action.String() == "closed" && hook.PullRequest.Merged {
		parts := strings.Split(hook.Repo.FullName, "/")

		err := o.applyBackports(l, "https://github.com", parts[0], parts[1], hook.PullRequest.Number)
		if err != nil {
			logrus.Errorf("Unable to apply backports %v", err)
		}
	}
}

func (o *Controller) addLabelToPr(l *logrus.Entry, host string, owner string, repo string, pr int, label string) error {
	k := service.NewKubernetes()
	u, t, err := k.GetCredentials(host)
	if err != nil {
		return err
	}

	l.Debugf("username=%s, password=XXX", u)

	s := service.NewScm(host, u, t)

	err = s.AddLabelToPr(owner, repo, pr, label)
	if err != nil {
		return err
	}

	return nil
}

func (o *Controller) addCommentToPr(l *logrus.Entry, host string, owner string, repo string, pr int, message string) error {
	k := service.NewKubernetes()
	u, t, err := k.GetCredentials(host)
	if err != nil {
		return err
	}

	l.Debugf("username=%s, password=XXX", u)

	s := service.NewScm(host, u, t)

	err = s.AddCommentToPr(owner, repo, pr, message)
	if err != nil {
		return err
	}

	return nil
}

func DetermineLabelsToAddFromComment(body string, lister Lister) ([]string, []string, error) {
	var messages []string
	var labels []string

	existingBranches, err := lister.Branches()
	if err != nil {
		return labels, messages, err
	}

	commentLines := strings.Split(body, "\n")
	for _, line := range commentLines {
		if strings.HasPrefix(line, "/backport") {
			branch := strings.TrimPrefix(line, "/backport ")
			branch = strings.TrimSpace(branch)

			if contains(existingBranches, branch) {
				labels = append(labels, fmt.Sprintf("%s%s", service.LabelPrefix, branch))
			} else {
				messages = append(messages, fmt.Sprintf("Unable to locate branch %s", branch))
			}
		}
	}

	return labels, messages, nil
}

func contains(list []string, item string) bool {
	for _, in := range list {
		if in == item {
			return true
		}
	}
	return false
}

func responseHTTPError(w http.ResponseWriter, statusCode int, response string) {
	logrus.WithFields(logrus.Fields{
		"response":    response,
		"status-code": statusCode,
	}).Info(response)
	http.Error(w, response, statusCode)
}

type Lister interface {
	Branches() ([]string, error)
}
