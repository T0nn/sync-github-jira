# GitHub-JIRA issue syncer

## Overview

GitHub-JIRA issue syncer is a stateless server for synchronizing issues one-side from GitHub to JIRA written in Go. It supports both full synchronization (use at first time) and incremental synchronization (real-time synchronization) using GitHub API v3, JIRA API v2 and GitHub webhook.

## Feature

- issues full synchronization from GitHub to JIRA for initial use
- real-time incremental issues synchronization using GitHub webhook
- synchronizing events of issues (open/close/reopen/edit/assign/unassign/label/unlabel), issue comments (create/delete/edit)
- complete support of GitHub-flavored Markdown to JIRA wiki transformation
- support of  repo map, assignee map, label map from GitHub to JIRA
- configuration using both toml file and command-line parameters

## Deployment / User guide / Configuration

### requirement

to build the project, run `GO111MODULE=on go build`

along with the binary, to run syncer, you also need following componenents:

- `ruby` to run Markdown transformation ruby script
- [commonmarker](https://github.com/gjtorikian/commonmarker) GitHub Flavored Markdown parser, install using `gem install commonmarker`
- `config.toml` config file

### configuration

``` toml
#example config file

JIRA-username = foo
JIRA-password = bar

JIRA-baseurl = "https://localhost:8080/"

# provided for larger GitHub API rate limit and accessing private repository, optional
github-username = foo
github-password = bar

# GitHub webhook payload listen port
listen-port = 8888

# last edited time of GitHub issues intend to synchronize
github-sincetime = "2018-09-29T00:00:00+08:00"

# per GitHub repo configuration
[repo]
  [repo.test] # GitHub repo name
    github-owner = "Tom-Xie" # GitHub repo owner name
    JIRA-project = "TEST" # target JIRA project key
    # JIRA-components = ["general"] # target JIRA project components field, optinal
    # label-map = {"enhancement"="enhancement", "question"="question"} # target JIRA project label field, optional
  [repo.another]
    github-owner = "Tom-Xie"
    JIRA-project = "ANOTHER"

# assignee map from GitHub login to JIRA username
[assignee]
  Test = "test@foo.bar"

[fix-versions]
  "TIDB" = "2.1"

```

### synchronization assumption

before synchronization, you should pay attention to the syncer's underneath assumption of GitHub and JIRA issues

- Some of the JIRA fields must not be hidden or removed, for example `GitHub ID`, otherwise the synchronization error would occur.
- Repo map, assignee map and label map are used to transform GitHub issue field to JIRA issue field. Syncer could ignore assignee map and label map (WIP) error, however, the repo map must be configured correctly.
- Time of the synchronized issues is the last edited time of issues. Using `github-sincetime` to configure it.
- GitHub account has the privilege of reading the configured repository
- JIRA account has the privillage of reading, writing, etc. the project and issues

### deployment procedure

- Deploy GitHub webhook and edit corresponded configure file entry, test webhook functionalitythis. This is optional depend on whether you intend to do incremental synchronization.
- Install `ruby` and `commanmarker`, test Markdown transformation locally using stdin and stdout.
- Configure the JIRA project issue settings according above synchronization assumption. Test basic synchronization using configuration with test GitHub repository.
- After thorough testing, deploy it running background in production enviroment.

### testing

**TODO**

## FAQ

- Why did synchronization fail?

Read the error message. Take another look of above description of requirement, configuration, assumption and deployment procedure. Also, pay attention to the JIRA issue field configuration.

- How full synchronization works?

**TODO**

- How incremental synchronization works?

**TODO**

- How to run in only full/incremental mode ?

Currently hack, which would be native support, only full: configure the `listen-port` to wrong port number, only incremental: modify `github-sincetime` to future time.

- How to configure repo map, assignee map and label map?

You could take a look of above example configure file. Moreover, the repo map is per GitHub repository to JIRA project configuration. The assignee map is GitHub user login to JIRA username map. And the label map is GitHub label to JIRA label map.

- How about the Markdown support?

It use ruby wrapper of the official GitHub flavored Markdown libary to parse comment and tranform into JIRA wiki. The output may look uggly sometimes due to the lack of express ability of JIRA wiki. Besides, we tranform ordered list in GitHub to unorderd list in JIRA.

- What is `use-lastsynctimefile` for?

Currently, it's not very useful, just ignore it.

## Design

**TODO**

webhook server from test-infra/prow/hook

fully sync idea from core-os/sync-JIRA

Markdown processing from blackfriday-confluence

file description
main.go
config.go
server.go

## Changelog

2018-10-08 v0.2: more GitHub webhook event support, full markdown transformation

2018-09-01 v0.1: initial release

## TODO

- support transition(close/reopen) of complex workflow
- online config setting (dangerous), only log level
- graceful shutdown, statistics and monitor
- support more GitHub webhook events
- checkpointing, periodically do full synchronization and save recent sync time to file
- enhance logging and error reporting
- use supervisor to start the program, such as systemd
- use mock server to add more testing
- add dry-run mode
- build the docker image for this project
- more authentacation method of GitHub and JIRA account

---

## Reference

**TODO**

add more reference and explanation

>
<https://github.com/kubernetes/test-infra/tree/master/prow/hook>

<https://github.com/coreos/issue-sync>

<https://github.com/gjtorikian/commonmarker>

<https://github.com/kentaro-m/blackfriday-confluence>