package webhook

import (
	"bytes"
	"fmt"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/driver/github"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"strings"
)

// Controller holds the command line arguments
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

// DefaultHandler responds to requests without a specific handler
func (o *Controller) DefaultHandler(w http.ResponseWriter, r *http.Request) {
	o.HandleWebhookRequests(w, r)
}

func (o *Controller) isReady() bool {
	return true
}

// HandleWebhookRequests handles incoming webhook events
func (o *Controller) HandleWebhookRequests(w http.ResponseWriter, r *http.Request) {
	o.handleWebhookOrPollRequest(w, r, "Webhook", func(scmClient *scm.Client, r *http.Request) (scm.Webhook, error) {
		return scmClient.Webhooks.Parse(r, o.secretFn)
	})
}

// handleWebhookOrPollRequest handles incoming events
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

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	scmClient := github.NewDefault()

	webhook, err := parseWebhook(scmClient, r)
	if err != nil {
		logrus.Warnf("failed to parse webhook: %s", err.Error())

		responseHTTPError(w, http.StatusInternalServerError, fmt.Sprintf("500 Internal Server Error: Failed to parse webhook: %s", err.Error()))
		return
	}
	if webhook == nil {
		logrus.Error("no webhook was parsed")

		responseHTTPError(w, http.StatusInternalServerError, "500 Internal Server Error: No webhook could be parsed")
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

// ProcessWebHook process a webhook
func (o *Controller) ProcessWebHook(l *logrus.Entry, webhook scm.Webhook) (*logrus.Entry, string, error) {
	repository := webhook.Repository()
	fields := map[string]interface{}{
		"Namespace": repository.Namespace,
		"Name":      repository.Name,
		"Branch":    repository.Branch,
		"Link":      repository.Link,
		"ID":        repository.ID,
		"Clone":     repository.Clone,
		"Webhook":   webhook.Kind(),
	}

	l = l.WithFields(fields)
	l.WithField("WebHook", fmt.Sprintf("%+v", webhook)).Info("webhook")

	_, ok := webhook.(*scm.PingHook)
	if ok {
		l.Info("received ping")
		return l, fmt.Sprintf("pong from backport"), nil
	}

	pushHook, ok := webhook.(*scm.PushHook)
	if ok {
		fields["Ref"] = pushHook.Ref
		fields["BaseRef"] = pushHook.BaseRef
		fields["Commit.Sha"] = pushHook.Commit.Sha
		fields["Commit.Link"] = pushHook.Commit.Link
		fields["Commit.Author"] = pushHook.Commit.Author
		fields["Commit.Message"] = pushHook.Commit.Message
		fields["Commit.Committer.Name"] = pushHook.Commit.Committer.Name

		l.Info("invoking Push handler")

		//o.handlePushEvent(l, pushHook)
		return l, "processed push hook", nil
	}
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

		//o.handlePullRequestEvent(l, prHook)
		return l, "processed PR hook", nil
	}
	branchHook, ok := webhook.(*scm.BranchHook)
	if ok {
		action := branchHook.Action
		ref := branchHook.Ref
		sender := branchHook.Sender
		fields["Action"] = action.String()
		fields["Ref.Sha"] = ref.Sha
		fields["Sender.Name"] = sender.Name

		l.Info("invoking branch handler")

		//o.handleBranchEvent(l, branchHook)
		return l, "processed branch hook", nil
	}
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

		//o.handleIssueCommentEvent(l, *issueCommentHook)
		return l, "processed issue comment hook", nil
	}
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

		l.Info("invoking Issue Comment handler")

		o.handlePullRequestCommentEvent(l, *prCommentHook)
		return l, "processed PR comment hook", nil
	}
	prReviewHook, ok := webhook.(*scm.ReviewHook)
	if ok {
		action := prReviewHook.Action
		fields["Action"] = action.String()
		pr := prReviewHook.PullRequest
		fields["PR.Number"] = pr.Number
		fields["PR.Ref"] = pr.Ref
		fields["PR.Sha"] = pr.Sha
		fields["PR.Title"] = pr.Title
		fields["PR.Body"] = pr.Body
		fields["Review.State"] = prReviewHook.Review.State
		fields["Reviewer.Name"] = prReviewHook.Review.Author.Name
		fields["Reviewer.Login"] = prReviewHook.Review.Author.Login
		fields["Reviewer.Avatar"] = prReviewHook.Review.Author.Avatar

		l.Info("invoking PR Review handler")

		return l, "processed PR review hook", nil
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

	commentBody := hook.Comment.Body
	commentLines := strings.Split(commentBody, "\n")
	for _, line := range commentLines {
		if strings.HasPrefix(line, "/backport") {
			l.Infof("we are interested in this line '%s'", line)
		}
	}
}

func responseHTTPError(w http.ResponseWriter, statusCode int, response string) {
	logrus.WithFields(logrus.Fields{
		"response":    response,
		"status-code": statusCode,
	}).Info(response)
	http.Error(w, response, statusCode)
}
