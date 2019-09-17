package main

import (
	jira "github.com/Tom-Xie/go-jira"
	githubGoogle "github.com/google/go-github/github"
	logrus "github.com/sirupsen/logrus"
)

func (s *Server) handleIssueCommentCreate(l *logrus.Entry, ic githubGoogle.IssueCommentEvent) error {

	// find correspond jira issue
	issueID := ic.GetIssue().GetID()
	projectKey := s.Config.RepoConfigMap[ic.GetRepo().GetName()].JiraProjectKey
	jiraIssue, err := s.findIssue(projectKey, issueID)
	if err != nil {
		return err
	}

	// prepare and format jira comment
	options := s.extractGithubIssueCommentOptions(*ic.GetComment())
	jiraComment := &jira.Comment{
		Body: s.jiraIssueCommentFormat(ic.GetComment().GetBody(), options),
	}

	// add jira comment
	_, _, err = s.jiraClient.Issue.AddComment(jiraIssue.ID, jiraComment)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) handleIssueCommentEdit(l *logrus.Entry, ic githubGoogle.IssueCommentEvent) error {

	// find correspond jira issue
	issueID := ic.GetIssue().GetID()
	projectKey := s.Config.RepoConfigMap[ic.GetRepo().GetName()].JiraProjectKey
	jiraIssue, err := s.findIssue(projectKey, issueID)
	if err != nil {
		return err
	}

	// find correspond jira comment
	result, err := s.findComment(jiraIssue.ID, ic.GetComment().GetID())
	if err != nil {
		return err
	}

	// prepare and format jira comment
	options := s.extractGithubIssueCommentOptions(*ic.GetComment())
	result.Body = s.jiraIssueCommentFormat(ic.GetComment().GetBody(), options)

	// update jira comment
	_, _, err = s.jiraClient.Issue.UpdateComment(jiraIssue.ID, &result)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) handleIssueCommentDelete(l *logrus.Entry, ic githubGoogle.IssueCommentEvent) error {

	// find correspond jira issue
	issueID := ic.GetIssue().GetID()
	projectKey := s.Config.RepoConfigMap[ic.GetRepo().GetName()].JiraProjectKey
	jiraIssue, err := s.findIssue(projectKey, issueID)
	if err != nil {
		return err
	}

	// find correspond jira issue
	result, err := s.findComment(jiraIssue.ID, ic.GetComment().GetID())
	if err != nil {
		return err
	}

	// delete jira comment
	err = s.jiraClient.Issue.DeleteComment(jiraIssue.ID, result.ID)
	if err != nil {
		return err
	}

	return nil
}
