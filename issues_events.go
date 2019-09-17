package main

import (
	"time"

	jira "github.com/Tom-Xie/go-jira"
	githubGoogle "github.com/google/go-github/github"
	logrus "github.com/sirupsen/logrus"
)

func (s *Server) handleIssueEventOpen(l *logrus.Entry, i githubGoogle.IssuesEvent) error {

	// prepare JIRA issue fields
	options := s.extractGithubIssueOptions(*i.GetIssue())
	jiraIssue := s.jiraIssueOpenFormat(i.GetIssue().GetID(), i.GetRepo().GetName(), i.GetIssue().GetTitle(), i.GetIssue().GetBody(), options)

	// create JIRA issue
	goto CreateIssueLabel
retryCreateLabel:
	jiraIssue.Fields.Assignee = nil
CreateIssueLabel:
	_, _, err := s.jiraClient.Issue.Create(&jiraIssue)
	if err != nil {
		l.Debug("error create JIRA issue")

		if reAssigneeError.MatchString(err.Error()) {
			l.Warn("retry create JIRA issue without assignee")
			goto retryCreateLabel
		} else {
			l.WithError(err).Error("error create JIRA issue")
		}
		return err
	}

	return nil
}

func (s *Server) handleIssueEventClosed(l *logrus.Entry, i githubGoogle.IssuesEvent) error {

	// find correspond jira issue
	issueID := i.GetIssue().GetID()
	repoConfig := s.Config.RepoConfigMap[i.GetRepo().GetName()]
	projectKey := repoConfig.JiraProjectKey
	jiraIssue, err := s.findIssue(projectKey, issueID)
	if err != nil {
		return err
	}

	// do JIRA transition to "Done"
	for _, transitionID := range repoConfig.TransitionMap[JiraTransitionDoneName] {
		_, err = s.jiraClient.Issue.DoTransition(jiraIssue.ID, transitionID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) handleIssueEventReopen(l *logrus.Entry, i githubGoogle.IssuesEvent) error {

	// find correspond jira issue
	issueID := i.GetIssue().GetID()
	repoConfig := s.Config.RepoConfigMap[i.GetRepo().GetName()]
	projectKey := repoConfig.JiraProjectKey
	jiraIssue, err := s.findIssue(projectKey, issueID)
	if err != nil {
		return err
	}

	// do JIRA transition to "To Do"
	for _, transitionID := range repoConfig.TransitionMap[JiraTransitionTodoName] {
		_, err = s.jiraClient.Issue.DoTransition(jiraIssue.ID, transitionID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) handleIssueEventEdit(l *logrus.Entry, i githubGoogle.IssuesEvent) (jira.Issue, error) {

	// find correspond jira issue
	issueID := i.GetIssue().GetID()
	projectKey := s.Config.RepoConfigMap[i.GetRepo().GetName()].JiraProjectKey
	jiraIssue, err := s.findIssue(projectKey, issueID)
	if err != nil {
		return jira.Issue{}, err
	}

	//  prepare JIRA issue fields
	options := s.extractGithubIssueOptions(*i.GetIssue())
	updateJiraIssue := s.jiraIssueUpdateFormat(jiraIssue.ID, jiraIssue.Key, i.GetIssue().GetTitle(), i.GetIssue().GetBody(), options)

	// update JIRA issue
	respJiraIssue, _, err := s.jiraClient.Issue.Update(&updateJiraIssue)
	if err != nil {
		return jira.Issue{}, err
	}

	return *respJiraIssue, nil
}

func (s *Server) handleIssueEventAssign(l *logrus.Entry, i githubGoogle.IssuesEvent) error {

	// in case label event with creating new issue
	time.Sleep(10 * time.Second)

	// find correspond jira issue
	issueID := i.GetIssue().GetID()
	projectKey := s.Config.RepoConfigMap[i.GetRepo().GetName()].JiraProjectKey
	jiraIssue, err := s.findIssue(projectKey, issueID)
	if err != nil {
		return err
	}

	// prepare JIRA assginee
	name, ok := s.Config.AssigneeMap[i.GetAssignee().GetLogin()]
	if !ok {
		l.Warn("assigned GitHub user login could not find corresponding jira user: ", i.GetAssignee().GetLogin())
		return nil
	}
	assignee := &jira.User{Name: name}

	// assign JIRA issue
	_, err = s.jiraClient.Issue.UpdateAssignee(jiraIssue.ID, assignee)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) handleIssueEventUnassign(l *logrus.Entry, i githubGoogle.IssuesEvent) error {

	// find correspond jira issue
	issueID := i.GetIssue().GetID()
	projectKey := s.Config.RepoConfigMap[i.GetRepo().GetName()].JiraProjectKey
	jiraIssue, err := s.findIssue(projectKey, issueID)
	if err != nil {
		return err
	}

	// prepare JIRA assginee
	intendUnassignee, ok := s.Config.AssigneeMap[i.GetAssignee().GetLogin()]
	if !ok {
		l.Warn("unassigned GitHub user login could not find corresponding jira user: ", i.GetAssignee().GetLogin())
		return nil
	}
	if intendUnassignee != jiraIssue.Fields.Assignee.Name {
		l.Debugf("intendUnassignee name not same as current jira assignee name %s %s", intendUnassignee, jiraIssue.Fields.Assignee.Name)
		return nil
	}
	assignee := &jira.User{}

	// unassign JIRA issue
	_, err = s.jiraClient.Issue.UpdateAssignee(jiraIssue.ID, assignee)
	if err != nil {
		return err
	}

	return nil
}

// TODO: use it both in event and presync and apply this pattern to other events

type eventFunc func(*logrus.Entry, *Server, githubGoogle.IssuesEvent, jira.Issue) error

func updateIssuetypeByLabel(l *logrus.Entry, s *Server, i githubGoogle.IssuesEvent, jiraIssue jira.Issue) error {

	githubRepoName := i.GetRepo().GetName()
	githubLabelName := i.GetLabel().GetName()
	intendIssuetypeName, ok := s.Config.RepoConfigMap[githubRepoName].IssueTypeLabelMap[githubLabelName]
	if !ok {
		l.Debugf("label '%s' not in '%s' label map ", githubLabelName, githubRepoName)
		return nil
	}

	if jiraIssue.Fields.Type.Name == intendIssuetypeName {
		l.Debugf("the issue already is the type %s", intendIssuetypeName)
		return nil
	}

	updateJiraIssue := jira.Issue{
		Key: jiraIssue.Key,
		Fields: &jira.IssueFields{
			Type: jira.IssueType{
				Name: intendIssuetypeName,
			},
		},
	}

	// update JIRA issue field
	_, _, err := s.jiraClient.Issue.Update(&updateJiraIssue)
	if err != nil {
		return err
	}

	return nil
}

func updateComponentByLabel(l *logrus.Entry, s *Server, i githubGoogle.IssuesEvent, jiraIssue jira.Issue) error {

	githubRepoName := i.GetRepo().GetName()
	githubLabelName := i.GetLabel().GetName()
	intendComponentName, ok := s.Config.RepoConfigMap[githubRepoName].ComponentLabelMap[githubLabelName]
	if !ok {
		l.Debugf("label '%s' not in '%s' label map ", githubLabelName, githubRepoName)
		return nil
	}

	for _, v := range jiraIssue.Fields.Components {
		if v.Name == intendComponentName {
			l.Debugf("the issue already in the component %s", intendComponentName)
			return nil
		}
	}

	updateJiraIssue := jira.Issue{
		Key: jiraIssue.Key,
		Fields: &jira.IssueFields{
			Components: []*jira.Component{
				&jira.Component{
					Name: intendComponentName,
				},
			},
		},
	}

	// update JIRA issue field
	_, _, err := s.jiraClient.Issue.Update(&updateJiraIssue)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) handleIssueEventLabel(l *logrus.Entry, i githubGoogle.IssuesEvent) error {

	// in case label event with creating new issue
	time.Sleep(10 * time.Second)

	// find correspond jira issue
	issueID := i.GetIssue().GetID()
	projectKey := s.Config.RepoConfigMap[i.GetRepo().GetName()].JiraProjectKey
	jiraIssue, err := s.findIssue(projectKey, issueID)
	if err != nil {
		return err
	}

	labelEventFunc := map[string]eventFunc{
		"updateIssuetypeByLabel": updateIssuetypeByLabel,
		"updateComponentByLabel": updateComponentByLabel,
	}

	var errReturn error
	for name, f := range labelEventFunc {
		logByFunc := l.WithField("label-event-func", name)
		if err := f(logByFunc, s, i, jiraIssue); err != nil {
			logByFunc.WithError(err).Error("error when running")
			errReturn = err
		}
	}
	return errReturn

}

func resetIssuetypeByUnlabel(l *logrus.Entry, s *Server, i githubGoogle.IssuesEvent, jiraIssue jira.Issue) error {

	githubRepoName := i.GetRepo().GetName()
	githubLabelName := i.GetLabel().GetName()
	intendIssuetypeName, ok := s.Config.RepoConfigMap[githubRepoName].IssueTypeLabelMap[githubLabelName]
	if !ok {
		l.Debugf("label '%s' not in '%s' label map ", githubLabelName, githubRepoName)
		return nil
	}

	if jiraIssue.Fields.Type.Name != intendIssuetypeName {
		l.Debugf("the issue is not the type %s", intendIssuetypeName)
		return nil
	}

	// TODO: make the deafult configuration configurable
	updateJiraIssue := jira.Issue{
		Key: jiraIssue.Key,
		Fields: &jira.IssueFields{
			Type: jira.IssueType{
				Name: s.Config.RepoConfigMap[githubRepoName].JiraIssueType,
			},
		},
	}

	// update JIRA issue field
	_, _, err := s.jiraClient.Issue.Update(&updateJiraIssue)
	if err != nil {
		return err
	}

	return nil
}

func resetComponentByUnlabel(l *logrus.Entry, s *Server, i githubGoogle.IssuesEvent, jiraIssue jira.Issue) error {

	githubRepoName := i.GetRepo().GetName()
	githubLabelName := i.GetLabel().GetName()
	intendComponentName, ok := s.Config.RepoConfigMap[githubRepoName].ComponentLabelMap[githubLabelName]
	if !ok {
		l.Debugf("label '%s' not in '%s' label map ", githubLabelName, githubRepoName)
		return nil
	}

	var found bool
	var components []*jira.Component
	for _, v := range jiraIssue.Fields.Components {
		if v.Name == intendComponentName {
			found = true
		} else {
			components = append(components, v)
		}
	}
	if !found {
		l.Debugf("the issue not in the component %s", intendComponentName)
		return nil
	}

	if len(components) == 0 {
		// TODO: make the deafult configuration configurable
		components = append(components, &jira.Component{Name: "general"})
	}

	updateJiraIssue := jira.Issue{
		Key: jiraIssue.Key,
		Fields: &jira.IssueFields{
			Components: components,
		},
	}

	// update JIRA issue field
	_, _, err := s.jiraClient.Issue.Update(&updateJiraIssue)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) handleIssueEventUnlabel(l *logrus.Entry, i githubGoogle.IssuesEvent) error {

	// find correspond jira issue
	issueID := i.GetIssue().GetID()
	projectKey := s.Config.RepoConfigMap[i.GetRepo().GetName()].JiraProjectKey
	jiraIssue, err := s.findIssue(projectKey, issueID)
	if err != nil {
		return err
	}

	labelEventFunc := map[string]eventFunc{
		"resetIssuetypeByUnlabel": resetIssuetypeByUnlabel,
		"resetComponentByUnlabel": resetComponentByUnlabel,
	}

	var errReturn error
	for name, f := range labelEventFunc {
		logByFunc := l.WithField("unlabel-event-func", name)
		if err := f(logByFunc, s, i, jiraIssue); err != nil {
			logByFunc.WithError(err).Error("error when running")
			errReturn = err
		}
	}
	return errReturn
}
