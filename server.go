package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	jira "github.com/Tom-Xie/go-jira"
	githubGoogle "github.com/google/go-github/github"

	logrus "github.com/sirupsen/logrus"
)

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate handles.
type Server struct {
	githubClient *githubGoogle.Client
	jiraClient   *jira.Client

	// how to save Config, global conf with local client conf?
	Config *Config

	// Tracks running handlers for graceful shutdown
	// wg sync.WaitGroup
}

func newServer(Config *Config) (*Server, error) {

	githubTransport := &githubGoogle.BasicAuthTransport{
		Username: Config.GithubUsername,
		Password: Config.GithubPassword,
	}
	githubClient := githubGoogle.NewClient(githubTransport.Client())

	jiraTransport := jira.BasicAuthTransport{
		Username: Config.JiraUsername,
		Password: Config.JiraPassword,
	}
	// err only happens when url is wrong, we gaurantee the url in the Configuration
	// reading phase, which frees us from err checking
	jiraClient, err := jira.NewClient(jiraTransport.Client(), Config.JiraBaseURL)
	if err != nil {
		return nil, err
	}

	logrus.Debug("start get JIRA custom fields")

	// get custom JIRA custom field ID and save it in Config
	Config.FieldIDs, err = getJiraFiledIDs(jiraClient)
	if err != nil {
		return nil, err
	}
	for k, v := range Config.FieldIDs {
		logrus.Debugf("%v: %v", k, v)
	}

	logrus.Debug("finish get JIRA custom fields")

	s := &Server{
		githubClient: githubClient,
		jiraClient:   jiraClient,
		Config:       Config,
	}
	return s, err
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok := validateWebhook(w, r)
	if !ok {
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")

	if err := s.demuxEvent(eventType, eventGUID, payload, r.Header); err != nil {
		logrus.WithError(err).Error("Error parsing event.")
	}
}

func validateWebhook(w http.ResponseWriter, r *http.Request) (string, string, []byte, bool) {
	defer r.Body.Close()

	// Our health check uses GET, so just kick back a 200.
	if r.Method == http.MethodGet {
		logrus.WithFields(logrus.Fields{
			"req": r,
		}).Debug()
		return "", "", nil, false
	}

	// Header checks: It must be a POST with an event type and a signature.
	if r.Method != http.MethodPost {
		resp := "405 Method not allowed"
		logrus.WithFields(logrus.Fields{
			"resp": resp,
		}).Debug()
		http.Error(w, resp, http.StatusMethodNotAllowed)
		return "", "", nil, false
	}
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		resp := "400 Bad Request: Missing X-GitHub-Event Header"
		logrus.WithFields(logrus.Fields{
			"resp": resp,
		}).Debug()
		http.Error(w, resp, http.StatusBadRequest)
		return "", "", nil, false
	}
	eventGUID := r.Header.Get("X-GitHub-Delivery")
	if eventGUID == "" {
		resp := "400 Bad Request: Missing X-GitHub-Delivery Header"
		logrus.WithFields(logrus.Fields{
			"resp": resp,
		}).Debug()
		http.Error(w, resp, http.StatusBadRequest)
		return "", "", nil, false
	}
	contentType := r.Header.Get("content-type")
	if contentType != "application/json" {
		resp := "400 Bad Request: Hook only accepts content-type: application/json - please reConfigure this hook on GitHub"
		logrus.WithFields(logrus.Fields{
			"resp": resp,
		}).Debug()
		http.Error(w, resp, http.StatusBadRequest)
		return "", "", nil, false
	}

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		resp := "500 Internal Server Error: Failed to read request body"
		logrus.WithFields(logrus.Fields{
			"resp": resp,
		}).Debug()
		http.Error(w, resp, http.StatusInternalServerError)
		return "", "", nil, false
	}

	return eventType, eventGUID, payload, true
}

func (s *Server) demuxEvent(eventType, eventGUID string, payload []byte, h http.Header) error {
	l := logrus.WithFields(logrus.Fields{
		"event-type": eventType,
		"event-GUID": eventGUID,
	},
	)

	switch eventType {
	case "issues":
		var i githubGoogle.IssuesEvent
		if err := json.Unmarshal(payload, &i); err != nil {
			return err
		}
		s.handleIssueEvent(l, i)
	case "issue_comment":
		var ic githubGoogle.IssueCommentEvent
		if err := json.Unmarshal(payload, &ic); err != nil {
			return err
		}
		s.handleIssueCommentEvent(l, ic)
	default:
		l.WithFields(logrus.Fields{
			"event-type": eventType,
		}).Warn("Unsupported type")
		return errors.New("Unsupported type")
	}

	return nil
}

func (s *Server) handleIssueEvent(l *logrus.Entry, i githubGoogle.IssuesEvent) {
	l = l.WithFields(logrus.Fields{
		"org":          i.GetRepo().GetOwner().GetLogin(),
		"repo":         i.GetRepo().GetName(),
		"pr":           i.GetIssue().GetNumber(),
		"author":       i.GetIssue().GetUser().GetLogin(),
		"url":          i.GetIssue().GetHTMLURL(),
		"event-action": i.GetAction(),
	})
	l.Debugf("Issue %s.", i.GetAction())

	if i.GetIssue().IsPullRequest() {
		l.Infof("not handle pull request issue")
		return
	}

	if err := s.demuxIssueEvent(l, i); err != nil {
		l.WithError(err).Error("Error handling IssueEvent.")
	}

}

func (s *Server) handleIssueCommentEvent(l *logrus.Entry, ic githubGoogle.IssueCommentEvent) {
	l = l.WithFields(logrus.Fields{
		"org":          ic.GetRepo().GetOwner().GetLogin(),
		"repo":         ic.GetRepo().GetName(),
		"pr":           ic.GetIssue().GetNumber(),
		"author":       ic.GetComment().GetUser().GetLogin(),
		"url":          ic.GetComment().GetHTMLURL(),
		"event-action": ic.GetAction(),
	})
	l.Debugf("Issue comment %s.", ic.GetAction())

	if ic.GetIssue().IsPullRequest() {
		l.Infof("not handle pull request issue")
		return
	}

	if err := s.demuxIssueCommentEvent(l, ic); err != nil {
		l.WithError(err).Error("Error handling IssueCommentEvent.")
	}

}

// demuxIssueEvent dispatches different github issue events to different handle function
func (s *Server) demuxIssueEvent(l *logrus.Entry, i githubGoogle.IssuesEvent) error {
	var err error
	switch i.GetAction() {
	case "opened":
		err = s.handleIssueEventOpen(l, i)
	case "closed":
		err = s.handleIssueEventClosed(l, i)
	case "reopened":
		err = s.handleIssueEventReopen(l, i)
	case "edited":
		_, err = s.handleIssueEventEdit(l, i)
	case "assigned":
		err = s.handleIssueEventAssign(l, i)
	case "unassigned":
		err = s.handleIssueEventUnassign(l, i)
	case "labeled":
		err = s.handleIssueEventLabel(l, i)
	case "unlabeled":
		err = s.handleIssueEventUnlabel(l, i)
	default:
	}
	return err
}

// demuxIssueCommentEvent dispatches different github issue comments events to different handle function
func (s *Server) demuxIssueCommentEvent(l *logrus.Entry, ic githubGoogle.IssueCommentEvent) error {
	var err error
	switch ic.GetAction() {
	case "created":
		err = s.handleIssueCommentCreate(l, ic)
	case "edited":
		err = s.handleIssueCommentEdit(l, ic)
	case "deleted":
		err = s.handleIssueCommentDelete(l, ic)
	default:
	}
	return err
}
