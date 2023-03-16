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

	logrus.Infof("raw event %s", string(bodyBytes))

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
		return l, "pong from backport", nil
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

		o.handlePullRequestEvent(l, prHook)
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

		o.handleIssueCommentEvent(l, *issueCommentHook)
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

	body := hook.Comment.Body
	o.handleComment(l, body)
}

func (o *Controller) handleIssueCommentEvent(l *logrus.Entry, hook scm.IssueCommentHook) {
	l.Infof("handling comment on Issue %d", hook.Issue.Number)
	l.Infof("new comment '%s'", hook.Comment.Body)

	body := hook.Comment.Body
	o.handleComment(l, body)
}

func (o *Controller) handleComment(l *logrus.Entry, body string) {
	commentLines := strings.Split(body, "\n")
	for _, line := range commentLines {
		if strings.HasPrefix(line, "/backport") {
			l.Infof("we are interested in this line '%s'", line)
		}
	}

	// FIXME this should not be here
	s := service.NewService()
	u, _, err := s.GetCredentials("https://github.com")
	l.Infof("username=%s, password=XXX, err=%v", u, err)
}

func (o *Controller) handlePullRequestEvent(l *logrus.Entry, hook *scm.PullRequestHook) {
	// msg=webhook
	// Branch=main
	// Clone="https://github.com/garethjevans/backport.git"
	// ID=613319709
	// Link="https://github.com/garethjevans/backport"
	// Name=backport
	// Namespace=garethjevans
	// WebHook="&{Action:closed Repo:{ID:613319709 Namespace:garethjevans Name:backport FullName:garethjevans/backport Perm:<nil> Branch:main Private:false Archived:false Clone:https://github.com/garethjevans/backport.git CloneSSH:git@github.com:garethjevans/backport.git Link:https://github.com/garethjevans/backport Created:0001-01-01 00:00:00 +0000 UTC Updated:0001-01-01 00:00:00 +0000 UTC} Label:{ID:0 URL: Name: Description: Color:} PullRequest:{Number:1 Title:Update README.md Body: Labels:[] Sha:32a5035217faf1ca942b1068b6cbdcf8de2010b0 Ref:refs/pull/1/head Source:garethjevans-patch-1 Target:main Base:{Ref:main Sha:7a3d50f1224b213351cd261c0367206e72ebb0e6 Repo:{ID:613319709 Namespace:garethjevans Name:backport FullName:garethjevans/backport Perm:0xc0002cf099 Branch:main Private:false Archived:false Clone:https://github.com/garethjevans/backport.git CloneSSH:git@github.com:garethjevans/backport.git Link:https://github.com/garethjevans/backport Created:2023-03-13 10:45:28 +0000 UTC Updated:2023-03-13 10:48:57 +0000 UTC}} Head:{Ref:garethjevans-patch-1 Sha:32a5035217faf1ca942b1068b6cbdcf8de2010b0 Repo:{ID:613319709 Namespace:garethjevans Name:backport FullName:garethjevans/backport Perm:0xc0002cf0c9 Branch:main Private:false Archived:false Clone:https://github.com/garethjevans/backport.git CloneSSH:git@github.com:garethjevans/backport.git Link:https://github.com/garethjevans/backport Created:2023-03-13 10:45:28 +0000 UTC Updated:2023-03-13 10:48:57 +0000 UTC}} Fork:garethjevans/backport State:closed Closed:true Draft:false Merged:true Mergeable:false Rebaseable:false MergeableState: MergeSha:e2268af2b3ef876f856a2baf5b47ac32f8d3a5de Author:{ID:158150 Login:garethjevans Name: Email: Avatar:https://avatars.githubusercontent.com/u/158150?v=4 Link:https://github.com/garethjevans IsAdmin:false Created:0001-01-01 00:00:00 +0000 UTC Updated:0001-01-01 00:00:00 +0000 UTC} Assignees:[] Reviewers:[] Milestone:{Number:0 ID:0 Title: Description: Link: State: DueDate:<nil>} Created:2023-03-14 16:51:47 +0000 UTC Updated:2023-03-15 16:39:26 +0000 UTC Link:https://github.com/garethjevans/backport/pull/1 DiffLink:https://github.com/garethjevans/backport/pull/1.diff} Sender:{ID:158150 Login:garethjevans Name: Email: Avatar:https://avatars.githubusercontent.com/u/158150?v=4 Link:https://github.com/garethjevans IsAdmin:false Created:0001-01-01 00:00:00 +0000 UTC Updated:0001-01-01 00:00:00 +0000 UTC} Changes:{Base:{Ref:{From:} Sha:{From:} Repo:{ID: Namespace: Name: FullName: Perm:<nil> Branch: Private:false Archived:false Clone: CloneSSH: Link: Created:0001-01-01 00:00:00 +0000 UTC Updated:0001-01-01 00:00:00 +0000 UTC}}} GUID:f34dac60-c34f-11ed-8673-b8ca42b8009f Installation:<nil>}" Webhook=pull_request
}

func responseHTTPError(w http.ResponseWriter, statusCode int, response string) {
	logrus.WithFields(logrus.Fields{
		"response":    response,
		"status-code": statusCode,
	}).Info(response)
	http.Error(w, response, statusCode)
}
