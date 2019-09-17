package main

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"

	jira "github.com/Tom-Xie/go-jira"
	logrus "github.com/sirupsen/logrus"
)

// JiraStatusDoneName represent the JIRA Done issue status name
// this could be varied across different project, need further consideration
const JiraStatusDoneName = "Done"

// JiraTransitionDoneName represent the JIRA Done transition Name
const JiraTransitionDoneName = "Done"

// JiraTransitionTodoName represent the JIRA To Do transition Name
const JiraTransitionTodoName = "To Do"

var reAssigneeError = regexp.MustCompile(`assignee.*User.*does not exist.`)
var jCommentIDRegex = regexp.MustCompile("^Comment \\[\\(ID (\\d+)\\)\\|")

func (s *Server) findIssue(projectKey string, issueID int64) (jira.Issue, error) {
	githubIssueFieldKey, _ := s.Config.getFieldKey(gitHubID)
	jql := fmt.Sprintf("project='%s' AND cf[%s] = %s",
		projectKey, githubIssueFieldKey, strconv.FormatInt(issueID, 10))

	jiraIssue, resp, err := s.jiraClient.Issue.Search(jql, nil)
	if err != nil {
		return jira.Issue{}, err
	}
	resp.Body.Close()

	if len(jiraIssue) == 0 {
		return jira.Issue{}, errors.New("Issue not exists")
	}

	return jiraIssue[0], nil
}

func doFindComment(jiraComments []*jira.Comment, githubCommentID int64) (result jira.Comment, found bool) {
	for _, jiraComment := range jiraComments {
		if jCommentIDRegex.MatchString(jiraComment.Body) {
			matches := jCommentIDRegex.FindStringSubmatch(jiraComment.Body)
			id, _ := strconv.ParseInt(matches[1], 10, 64)
			if githubCommentID == id {
				found = true
				result = *jiraComment
				break
			}
		}
	}

	return
}

func (s *Server) findComment(jiraIssueID string, githubCommentID int64) (jira.Comment, error) {

	// get JIRA issue comments
	commentsAPIEndpoint := fmt.Sprintf("rest/api/2/issue/%s/comment", jiraIssueID)
	req, err := s.jiraClient.NewRequest("GET", commentsAPIEndpoint, nil)
	if err != nil {
		return jira.Comment{}, err
	}
	jiraComments := new(jira.Comments)
	_, err = s.jiraClient.Do(req, jiraComments)
	if err != nil {
		return jira.Comment{}, err
	}

	result, found := doFindComment(jiraComments.Comments, githubCommentID)

	if !found {
		return jira.Comment{}, errors.New("Corresponded JIRA comment not exists")
	}

	return result, err
}

func jiraMarkdownTransform(githubIssueBodyOld string) (string, error) {

	maxLength := 30000 // jira max length 32767, we reserve some for additional information text

	cmd := exec.Command("./markdown.rb")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		logrus.WithError(err).Error("jiraMarkdownTransform redirect StdinPipe error occur")
		return shrinkString(githubIssueBodyOld, maxLength), err
	}
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, githubIssueBodyOld)
	}()
	githubIssueBodyNewByte, err := cmd.Output()
	if err != nil {
		logrus.WithError(err).Error("jiraMarkdownTransform run ruby markdown process error occur")
		return shrinkString(githubIssueBodyOld, maxLength), err
	}

	githubIssueBodyNew := shrinkString(string(githubIssueBodyNewByte), maxLength)

	return githubIssueBodyNew, nil
}

func (s *Server) jiraIssueBodyFormat(githubIssueBody string, options githubIssueOptions) string {

	footnotes := fmt.Sprintf(
		"Create issue [(#%s)|%s] from GitHub user [%s|%s]",
		options.githubIssueNumber,
		options.githubIssueLink,
		options.githubIssueUserLogin,
		options.githubIssueUserLink)
	if len(options.githubIssueUserName) > 0 {
		footnotes = fmt.Sprintf("%s (%s)", footnotes, options.githubIssueUserName)
	}

	jiraIssueBody, _ := jiraMarkdownTransform(githubIssueBody)

	ret := fmt.Sprintf(
		"%s\n%s\n%s at %s",
		// jiraURLLinkTransform(jiraImageLinkTransform(jiraCodeStyleTransform(githubIssueBody))),
		jiraIssueBody,
		"\n----\n",
		footnotes,
		options.githubIssueTime,
	)

	return ret
}

func (s *Server) jiraIssueOpenFormat(githubIssueID int64, repoName, githubIssueTitle, githubIssueBody string, options githubIssueOptions, others ...interface{}) jira.Issue {

	var components []*jira.Component
	for _, v := range s.Config.RepoConfigMap[repoName].JiraComponents {
		components = append(components, &jira.Component{Name: v})
	}

	var fixVersions []*jira.Version
	for _, v := range s.Config.FixVersions[s.Config.RepoConfigMap[repoName].JiraProjectKey] {
		fixVersions = append(fixVersions, &jira.Version{Name: v})
	}
	// fixVersions = append(fixVersions, &jira.Version{Name: s.Config.FixVersions[s.Config.RepoConfigMap[repoName].JiraProjectKey]})

	var affectsVersions []*jira.Version
	for _, v := range s.Config.AffectsVersions[s.Config.RepoConfigMap[repoName].JiraProjectKey] {
		affectsVersions = append(affectsVersions, &jira.Version{Name: v})
	}

	var assignee *jira.User
	if name, ok := s.Config.AssigneeMap[options.githubIssueAssigneeLogin]; ok {
		assignee = &jira.User{Name: name}
	}

	var labels = []string{"github"}
	// for _, v := range options.githubLabels {
	// 	if jiraLabel, ok := s.Config.RepoConfigMap[repoName].LabelMap[v.GetName()]; ok {
	// 		labels = append(labels, jiraLabel)
	// 	}
	// }

	fields := jira.IssueFields{
		Type: jira.IssueType{
			Name: s.Config.RepoConfigMap[repoName].JiraIssueType, // need to determine the issue type
		},
		Project: jira.Project{
			Key: s.Config.RepoConfigMap[repoName].JiraProjectKey,
		},
		Components:      components,
		FixVersions:     fixVersions,
		AffectsVersions: affectsVersions,
		Labels:          labels,
		Assignee:        assignee,
		Summary:         githubIssueTitle,
		Description:     s.jiraIssueBodyFormat(githubIssueBody, options),
		Unknowns:        map[string]interface{}{},
	}

	// set JIRA custom field "GitHub ID" = issue.id
	githubIssueFieldID, err := s.Config.getFieldID(gitHubID)
	if err == nil {
		fields.Unknowns[githubIssueFieldID] = githubIssueID
	}

	// TODO: make following code more general, by using per project configuration

	// issue custom field githubURL only appears in JIRA project TIKV
	if s.Config.RepoConfigMap[repoName].JiraProjectKey == "TIKV" {
		githubURLFieldID, err := s.Config.getFieldID(gitHubURL)
		if err == nil {
			fields.Unknowns[githubURLFieldID] = options.githubIssueLink
		}
	}

	jiraIssue := jira.Issue{
		Fields: &fields,
	}

	return jiraIssue
}

func (s *Server) jiraIssueUpdateFormat(jiraIssueID, jiraIssueKey, githubIssueTitle, githubIssueBody string, options githubIssueOptions, others ...interface{}) jira.Issue {

	fields := jira.IssueFields{
		Summary:     githubIssueTitle,
		Description: s.jiraIssueBodyFormat(githubIssueBody, options),
	}

	jiraIssue := jira.Issue{
		Fields: &fields,
		Key:    jiraIssueKey,
		ID:     jiraIssueID,
	}

	return jiraIssue
}

func (s *Server) jiraIssueCommentFormat(githubIssueCommentBody string, options githubIssueCommentOptions, others ...interface{}) string {

	// note: use jira html link format to add link [name|link]
	footnotes := fmt.Sprintf(
		"Comment [(ID %s)|%s] from GitHub user [%s|%s]",
		options.githubIssueCommentID,
		options.githubIssueCommentLink,
		options.githubIssueCommentUserLogin,
		options.githubIssueCommentUserLink)
	if len(options.githubIssueCommentUserName) > 0 {
		footnotes = fmt.Sprintf("%s (%s)", footnotes, options.githubIssueCommentUserName)
	}

	jiraIssueCommentBody, _ := jiraMarkdownTransform(githubIssueCommentBody)

	ret := fmt.Sprintf(
		"%s at %s\n%s\n%s\n",
		footnotes,
		options.githubIssueCommentTime,
		"\n----\n",
		// jiraURLLinkTransform(jiraImageLinkTransform(jiraCodeStyleTransform(githubIssueCommentBody))),
		jiraIssueCommentBody,
	)
	return ret
}
