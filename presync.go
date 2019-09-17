package main

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sync"

	jira "github.com/Tom-Xie/go-jira"
	githubGoogle "github.com/google/go-github/github"
	logrus "github.com/sirupsen/logrus"
)

const lastSyncTimeFileName = "last_sync_time.txt"

func (s *Server) compareSyncIssues(l *logrus.Entry) error {

	s.readLastSyncTime(l)

	// sync all repos in parallel
	var errReturn error
	var wgRepo sync.WaitGroup
	for repoName := range s.Config.RepoConfigMap {

		// find all github issues in repo
		allGithubIssues, err := s.getGithubIssuesByRepo(l, s.Config.RepoConfigMap[repoName].GithubOwner, repoName)
		if err != nil {
			l.WithError(err).Errorf("getGithubIssuesByRepo error occur of %s", repoName)
			errReturn = err
			continue // error only affects this repo, just pass
		}
		l.WithFields(logrus.Fields{
			"repoName": repoName,
			"number":   len(allGithubIssues),
		}).Debug("finish get all github issues")

		wgRepo.Add(1)

		go func(l *logrus.Entry, repoName string) {

			defer wgRepo.Done()

			// create all github corresponding issue in squential(order)
			for _, githubIssue := range allGithubIssues {

				_, err := s.findIssue(s.Config.RepoConfigMap[repoName].JiraProjectKey, githubIssue.GetID())

				// TODO: mark not exists/created issue and pass the following issue updating, and remove below findIssue()
				if err != nil {
					if err.Error() == "Issue not exists" {
						// create new JIRA issue as not found the githubIssue
						_, err = s.compareSyncIssuesCreate(l, *githubIssue, repoName)
						if err != nil {
							l.WithError(err).Error("error with compareSyncIssuesCreate")
							continue // error only affects this issue, just pass
						}
					} else {
						l.WithError(err).Error("error with findIssue when compareSyncIssuesCreate")
						continue // just pass current issue
					}
				}
			}

			// update all issues and comments in parallel
			var wgIssue sync.WaitGroup
			for _, githubIssue := range allGithubIssues {

				wgIssue.Add(1)

				go func(l *logrus.Entry, githubIssue *githubGoogle.Issue) {
					defer wgIssue.Done()

					jiraIssue, err := s.findIssue(s.Config.RepoConfigMap[repoName].JiraProjectKey, githubIssue.GetID())
					if err != nil {
						l.WithError(err).Error("error with findIssue when compareSyncIssuesUpdate&compareSyncComments")
						return
					}

					u, _ := url.Parse(s.Config.JiraBaseURL)
					u.Path = path.Join(u.Path, "browse", jiraIssue.Key)
					l = l.WithFields(
						logrus.Fields{
							"githubIssueURL": githubIssue.GetHTMLURL(),
							"jiraIssueURL":   u.String(),
						})

					// update the corresponding JIRA issue according to github issue
					l.Debug("start compareSyncIssuesUpdate")
					err = s.compareSyncIssuesUpdate(l, jiraIssue, *githubIssue, repoName)
					if err != nil {
						l.WithError(err).Error("error with compareSyncIssuesUpdate")
						return
					}
					l.Debug("finish compareSyncIssuesUpdate")

					l = l.WithFields(
						logrus.Fields{
							"event-type": "compareSyncComments",
						})
					// both need to compare JIRA issue comments with GitHub issue comments
					l.Debug("start compareSyncComments")
					err = s.compareSyncComments(l, jiraIssue, *githubIssue, repoName)
					if err != nil {
						l.WithError(err).Error("error with compareSyncComments")
					}
					l.Debug("finish compareSyncComments")

				}(l, githubIssue)
			}

			wgIssue.Wait()

		}(l, repoName)

	}
	wgRepo.Wait()

	s.writeLastSyncTime(l)

	return errReturn
}

// compareSyncIssuesCreate is simlar to handleIssueEventOpen, however due to different formats of GitHub issue/issueEvent representations, we make this function instead of call handleIssueEventOpen() directly
func (s *Server) compareSyncIssuesCreate(l *logrus.Entry, githubIssue githubGoogle.Issue, repoName string) (jira.Issue, error) {
	l = l.WithFields(
		logrus.Fields{
			"githubIssueURL": githubIssue.GetHTMLURL(),
		})

	githubIssueID := githubIssue.GetID()
	githubIssueTitle := githubIssue.GetTitle()
	githubIssueBody := githubIssue.GetBody()
	githubIssueStatus := githubIssue.GetState()

	// prepare JIRA issue fields
	options := s.extractGithubIssueOptions(githubIssue)
	jiraIssue := s.jiraIssueOpenFormat(githubIssueID, repoName, githubIssueTitle, githubIssueBody, options)

	// check if githubIssueAssigneeLogin in AssigneeMap, for adding AssigneeMap later
	if _, ok := s.Config.AssigneeMap[options.githubIssueAssigneeLogin]; !ok && options.githubIssueAssigneeLogin != "" {
		l.Warn("GitHub user login not find: ", options.githubIssueAssigneeLogin)
	}

	l.Debug("finish prepare JIRA issue fields")

	// create JIRA issue
	goto CreateIssueLabel
retryCreateLabel:
	jiraIssue.Fields.Assignee = nil
CreateIssueLabel:
	respJiraIssue, resp, err := s.jiraClient.Issue.Create(&jiraIssue)
	if err != nil {
		if reAssigneeError.MatchString(err.Error()) {
			l.Warn("retry create JIRA issue without assignee: ", jiraIssue.Fields.Assignee.Name)
			goto retryCreateLabel
		} else {
			l.WithError(err).Warn("error create JIRA issue")
		}
		return jira.Issue{}, err
	}
	resp.Body.Close()

	// sync JIRA issue assignee speratelly, this approach maybe daunting ??

	// sync JIRA issue transition status, "To Do" to "Done"
	repoConfig := s.Config.RepoConfigMap[repoName]
	if githubIssueStatus == "closed" {
		for _, transitionID := range repoConfig.TransitionMap[JiraTransitionDoneName] {
			_, err = s.jiraClient.Issue.DoTransition(respJiraIssue.ID, transitionID)
			if err != nil {
				l.WithError(err).Error("JIRA issue transition to closed error")
				break
			}
		}
	}

	// sync jiraIssue issue type according to github label
	var issueTypes []string
	for _, label := range githubIssue.Labels {
		if name, ok := s.Config.RepoConfigMap[repoName].IssueTypeLabelMap[label.GetName()]; ok {
			issueTypes = append(issueTypes, name)
		}
	}
	if len(issueTypes) != 0 {
		updateJiraIssue := jira.Issue{
			Key: respJiraIssue.Key,
			Fields: &jira.IssueFields{
				Type: jira.IssueType{
					Name: issueTypes[0],
				},
			},
		}
		_, _, err := s.jiraClient.Issue.Update(&updateJiraIssue)
		if err != nil {
			l.WithError(err).Error("create JIRA issue change issueType by label error")
		}
	}

	// sync jiraIssue component according to github label
	var components []*jira.Component
	for _, label := range githubIssue.Labels {
		if name, ok := s.Config.RepoConfigMap[repoName].ComponentLabelMap[label.GetName()]; ok {
			components = append(components, &jira.Component{Name: name})
		}
	}

	if len(components) != 0 {
		updateJiraIssue := jira.Issue{
			Key: respJiraIssue.Key,
			Fields: &jira.IssueFields{
				Components: components,
			},
		}
		_, _, err := s.jiraClient.Issue.Update(&updateJiraIssue)
		if err != nil {
			l.WithError(err).Error("create JIRA issue change components by label error")
		}
	}

	l.Debug("finish compareSyncIssuesCreate")

	return *respJiraIssue, nil
}

// compareSyncIssuesUpdate has similar intention as above compareSyncIssuesCreate
func (s *Server) compareSyncIssuesUpdate(l *logrus.Entry, jiraIssue jira.Issue, githubIssue githubGoogle.Issue, repoName string) error {

	githubIssueTitle := githubIssue.GetTitle()
	githubIssueBody := githubIssue.GetBody()

	//  prepare JIRA issue fields
	options := s.extractGithubIssueOptions(githubIssue)
	updateJiraIssue := s.jiraIssueUpdateFormat(jiraIssue.ID, jiraIssue.Key, githubIssueTitle, githubIssueBody, options)

	// update JIRA issue
	_, resp, err := s.jiraClient.Issue.Update(&updateJiraIssue)
	if err != nil {
		return err
	}
	resp.Body.Close()

	// sync JIRA issue assignee, this approach maybe daunting
	var assignee *jira.User
	if name, ok := s.Config.AssigneeMap[options.githubIssueAssigneeLogin]; ok {
		assignee = &jira.User{Name: name}
	} else if options.githubIssueAssigneeLogin != "" {
		l.Warn("GitHub user login not find: ", options.githubIssueAssigneeLogin)
	}
	// if jira issue already have assignee, should we overwrite it ??
	_, err = s.jiraClient.Issue.UpdateAssignee(jiraIssue.ID, assignee)
	if err != nil {
		l.WithError(err).Warn("assign JIRA issue to user error ", options.githubIssueUserLogin)
	}

	// sync issue transition status
	repoConfig := s.Config.RepoConfigMap[repoName]
	if githubIssue.GetState() == "closed" {
		if jiraIssue.Fields.Status.StatusCategory.Name != JiraStatusDoneName {
			for _, transitionID := range repoConfig.TransitionMap[JiraTransitionDoneName] {
				_, err = s.jiraClient.Issue.DoTransition(jiraIssue.ID, transitionID)
				if err != nil {
					l.WithError(err).Error("JIRA issue transition to closed error")
					break
				}
			}
		}
	} else if githubIssue.GetState() == "open" {
		if jiraIssue.Fields.Status.StatusCategory.Name == JiraStatusDoneName {
			for _, transitionID := range repoConfig.TransitionMap[JiraTransitionTodoName] {
				_, err = s.jiraClient.Issue.DoTransition(jiraIssue.ID, transitionID)
				if err != nil {
					l.WithError(err).Error("JIRA issue transition to open error")
					break
				}
			}
		}
	}

	// sync jiraIssue issue type according to github label
	var issueTypes []string
	for _, label := range githubIssue.Labels {
		if name, ok := s.Config.RepoConfigMap[repoName].IssueTypeLabelMap[label.GetName()]; ok {
			issueTypes = append(issueTypes, name)
		}
	}

	var toUpdateIssueTypeName string
	if len(issueTypes) != 0 {
		var found bool
		for _, name := range issueTypes {
			if name == jiraIssue.Fields.Type.Name {
				found = true
				break
			}
		}
		if !found {
			toUpdateIssueTypeName = issueTypes[0]
		}

	} else {
		toUpdateIssueTypeName = s.Config.RepoConfigMap[repoName].JiraIssueType
	}

	if toUpdateIssueTypeName != "" {
		updateIssueTypeJiraIssue := jira.Issue{
			Key: jiraIssue.Key,
			Fields: &jira.IssueFields{
				Type: jira.IssueType{
					Name: toUpdateIssueTypeName,
				},
			},
		}
		_, _, err = s.jiraClient.Issue.Update(&updateIssueTypeJiraIssue)
		if err != nil {
			l.WithError(err).Error("update JIRA issue change issueType by label error")
		}
	}

	// sync jiraIssue component according to github label
	// ?? only S_GitHub or S_JIRA U S_GitHub
	var components []*jira.Component
	for _, label := range githubIssue.Labels {
		if name, ok := s.Config.RepoConfigMap[repoName].ComponentLabelMap[label.GetName()]; ok {
			components = append(components, &jira.Component{Name: name})
		}
	}
	if len(components) == 0 {
		for _, v := range s.Config.RepoConfigMap[repoName].JiraComponents {
			components = append(components, &jira.Component{Name: v})
		}
	}

	// ?? maybe update only if !same(jiraIssue.Fields.Components, components)
	updateComponentsJiraIssue := jira.Issue{
		Key: jiraIssue.Key,
		Fields: &jira.IssueFields{
			Components: components,
		},
	}
	_, _, err = s.jiraClient.Issue.Update(&updateComponentsJiraIssue)
	if err != nil {
		l.WithError(err).Error("update JIRA issue change components by label error")
	}

	l.Debug("finish compareSyncIssuesUpdate")

	return nil
}

func (s *Server) compareSyncComments(l *logrus.Entry, jiraIssue jira.Issue, githubIssue githubGoogle.Issue, repoName string) error {

	// we don't handle the return pagination temporarily, as the default return maxResults is 1048576 as https://internal.pingcap.net/jira/rest/api/2/issue/TIDB-1353/comment?startAt=0&maxResults=1048576
	// get JIRA issue comments
	commentsAPIEndpoint := fmt.Sprintf("rest/api/2/issue/%s/comment", jiraIssue.ID)
	req, err := s.jiraClient.NewRequest("GET", commentsAPIEndpoint, nil)
	if err != nil {
		return err
	}
	jiraComments := new(jira.Comments)
	resp, err := s.jiraClient.Do(req, jiraComments)
	if err != nil {
		return err
	}
	resp.Body.Close()

	// github comments number equals zero, still need to compare comments and delete
	if githubIssue.GetComments() == 0 {
		for _, jiraComment := range jiraComments.Comments {

			var jCommentIDRegex = regexp.MustCompile("^Comment \\[\\(ID (\\d+)\\)\\|")
			if !jCommentIDRegex.MatchString(jiraComment.Body) {
				continue
			}

			// correspond github issue comment is deleted
			// delete JIRA issue comment
			err = s.jiraClient.Issue.DeleteComment(jiraIssue.ID, jiraComment.ID)
			if err != nil {
				l.WithError(err).Warn("Delete JIRA comment error")
				continue
			}

		}

		return err
	}

	githubComments, err := s.getGithubIssueComments(s.Config.RepoConfigMap[repoName].GithubOwner, repoName, githubIssue.GetNumber())
	if err != nil {
		return err
	}

	// l.Debug("start traverse github Comments")

	// if github comments exits, there are three situations of jira issue comment, exits, not exits, not corresponded(not deal with it)
	for _, githubComment := range githubComments {

		result, found := doFindComment(jiraComments.Comments, githubComment.GetID())

		// situation 1: github issue comment has corresponding jira issue comment, update it
		if found {
			err = s.compareSyncCommentsUpdate(l, jiraIssue, result, *githubComment)
			if err != nil {
				l.WithError(err).Warn("compareSyncCommentsUpdate error")
				continue
			}
		} else {
			//  situation 2: github issue comment has not corresponding jira issue comment exists, create it
			err = s.compareSyncCommentsCreate(l, jiraIssue, *githubComment)
			if err != nil {
				l.WithError(err).Warn("compareSyncCommentsCreate error")
				continue
			}
		}
	}

	return err
}

// compareSyncCommentsCreate is simlar to handleIssueCommentCreate, however due to different formats of GitHub issue/issueEvent representations, we make this function instead of call handleIssueCommentCreate() directly
// could just use jiraIssue.ID to improve performance
func (s *Server) compareSyncCommentsCreate(l *logrus.Entry, jiraIssue jira.Issue, githubComment githubGoogle.IssueComment) error {

	githubCommentBody := githubComment.GetBody()
	options := s.extractGithubIssueCommentOptions(githubComment)
	jiraComment := &jira.Comment{
		Body: s.jiraIssueCommentFormat(githubCommentBody, options),
	}

	_, resp, err := s.jiraClient.Issue.AddComment(jiraIssue.ID, jiraComment)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

// compareSyncCommentsUpdate has similar intention as above compareSyncCommentsCreate
func (s *Server) compareSyncCommentsUpdate(l *logrus.Entry, jiraIssue jira.Issue, jiraComment jira.Comment, githubComment githubGoogle.IssueComment) error {

	result := jiraComment

	githubCommentBody := githubComment.GetBody()
	options := s.extractGithubIssueCommentOptions(githubComment)
	result.Body = s.jiraIssueCommentFormat(githubCommentBody, options)

	_, resp, err := s.jiraClient.Issue.UpdateComment(jiraIssue.ID, &result)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}
