package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"

	githubGoogle "github.com/google/go-github/github"
	logrus "github.com/sirupsen/logrus"
)

const commentDateFormat = "15:04 PM, January 2 2006"

type githubIssueOptions struct {
	githubIssueNumber        string
	githubIssueLink          string
	githubIssueUserLogin     string
	githubIssueUserLink      string
	githubIssueUserName      string
	githubIssueTime          string
	githubIssueAssigneeLogin string
	githubLabels             []githubGoogle.Label
}

func (s *Server) extractGithubIssueOptions(githubIssue githubGoogle.Issue) githubIssueOptions {

	options := githubIssueOptions{
		githubIssueNumber:        strconv.Itoa(githubIssue.GetNumber()),
		githubIssueLink:          githubIssue.GetHTMLURL(),
		githubIssueUserLogin:     githubIssue.GetUser().GetLogin(),
		githubIssueUserLink:      githubIssue.GetUser().GetHTMLURL(),
		githubIssueUserName:      githubIssue.GetUser().GetName(),
		githubIssueTime:          githubIssue.GetCreatedAt().In(s.Config.Loc).Format(commentDateFormat),
		githubIssueAssigneeLogin: githubIssue.GetAssignee().GetLogin(),
		githubLabels:             githubIssue.Labels,
	}

	return options
}

type githubIssueCommentOptions struct {
	githubIssueCommentID        string
	githubIssueCommentLink      string
	githubIssueCommentUserLogin string
	githubIssueCommentUserLink  string
	githubIssueCommentUserName  string
	githubIssueCommentTime      string
}

func (s *Server) extractGithubIssueCommentOptions(githubIssueComment githubGoogle.IssueComment) githubIssueCommentOptions {

	options := githubIssueCommentOptions{
		githubIssueCommentID:        strconv.FormatInt(githubIssueComment.GetID(), 10),
		githubIssueCommentLink:      githubIssueComment.GetHTMLURL(),
		githubIssueCommentUserLogin: githubIssueComment.GetUser().GetLogin(),
		githubIssueCommentUserLink:  githubIssueComment.GetUser().GetHTMLURL(),
		githubIssueCommentUserName:  githubIssueComment.GetUser().GetName(),
		githubIssueCommentTime:      githubIssueComment.GetCreatedAt().In(s.Config.Loc).Format(commentDateFormat),
	}

	return options
}

func (s *Server) getGithubIssuesByRepo(l *logrus.Entry, owner, repoName string) ([]*githubGoogle.Issue, error) {
	var allIssues []*githubGoogle.Issue
	ctx := context.Background()
	githubIssueListByRepoOptions := &githubGoogle.IssueListByRepoOptions{
		Since:     s.Config.GithubIssueSince,
		State:     "all",
		Sort:      "created",
		Direction: "asc",
		ListOptions: githubGoogle.ListOptions{
			Page:    1,
			PerPage: 100, // maxmium is 100
		},
	}
	for {
		l.Debugf("get github issues by repo per page %4d in %s", githubIssueListByRepoOptions.ListOptions.Page, repoName)
		issues, resp, err := s.githubClient.Issues.ListByRepo(ctx, owner, repoName, githubIssueListByRepoOptions)

		if err != nil {
			return nil, err
		}
		// only consider issue which is not pullrequest
		for _, i := range issues {
			if !i.IsPullRequest() {
				allIssues = append(allIssues, i)
			}
		}
		if resp.NextPage == 0 {

			resp.Body.Close()

			break
		}
		githubIssueListByRepoOptions.Page = resp.NextPage

		resp.Body.Close()

	}

	return allIssues, nil
}

func dumpGithubIssue(allIssues []*githubGoogle.Issue, owner, repoName string) error {
	b, err := json.Marshal(allIssues)
	if err != nil {
		fmt.Println(err)
		return err
	}
	if err := ioutil.WriteFile(fmt.Sprintf("%s.%s.json", owner, repoName), b, 0600); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

func (s *Server) getGithubIssueComments(owner, repoName string, number int) ([]*githubGoogle.IssueComment, error) {
	var allComments []*githubGoogle.IssueComment
	ctx := context.Background()
	githubIssueListCommentsOptions := &githubGoogle.IssueListCommentsOptions{
		Since:     s.Config.GithubIssueSince,
		Sort:      "created",
		Direction: "asc",
		ListOptions: githubGoogle.ListOptions{
			Page:    1,
			PerPage: 100, // maxmium is 100
		},
	}
	for {
		comments, resp, err := s.githubClient.Issues.ListComments(ctx, owner, repoName, number, githubIssueListCommentsOptions)

		if err != nil {
			return nil, err
		}
		allComments = append(allComments, comments...)
		if resp.NextPage == 0 {
			resp.Body.Close()
			break
		}
		githubIssueListCommentsOptions.Page = resp.NextPage

		resp.Body.Close()

	}
	return allComments, nil
}
