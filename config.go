package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	jira "github.com/Tom-Xie/go-jira"
	"github.com/juju/errors"
	logrus "github.com/sirupsen/logrus"
)

//RepoConfig store repo related information
type RepoConfig struct {
	GithubOwner       string              `toml:"github-owner" json:"github-owner"`
	JiraProjectKey    string              `toml:"jira-project" json:"jira-project"`
	JiraComponents    []string            `toml:"jira-components,omitempty" json:"jira-components,omitempty"`
	JiraIssueType     string              `toml:"jira-issuetype,omitempty" json:"jira-issuetype,omitempty"`
	IssueTypeLabelMap map[string]string   `toml:"issuetype-label-map,omitempty" json:"issuetype-label-map,omitempty"`
	ComponentLabelMap map[string]string   `toml:"component-label-map,omitempty" json:"component-label-map,omitempty"`
	TransitionMap     map[string][]string `toml:"transition-map,omitempty" json:"transition-map,omitempty"`
}

// Config is config for the server
type Config struct {
	*flag.FlagSet

	configFile string
	LogLevel   string `toml:"log-level" json:"log-level"`

	Version bool `json:"-"`

	ListenPort     int    `toml:"listen-port" json:"listen-port"`
	GithubUsername string `toml:"github-username" json:"github-username"`
	GithubPassword string `toml:"github-password" json:"github-password"`
	JiraUsername   string `toml:"jira-username" json:"jira-username"`
	JiraPassword   string `toml:"jira-password" json:"jira-password"`
	JiraBaseURL    string `toml:"jira-baseurl" json:"jira-baseurl"`

	DoPreSync bool `toml:"do-presync" json:"do-presync"`

	UseLastSyncTimeFile bool `toml:"use-lastsynctimefile" json:"use-lastsynctimefile"`

	GithubIssueSince time.Time `toml:"github-sincetime" json:"github-sincetime"`

	Loc *time.Location

	// GitHub repo name to JIRA project config map
	RepoConfigMap map[string]RepoConfig `toml:"repo" json:"repo"`

	// JIRA related map
	FixVersions     map[string][]string `toml:"fix-versions,omitempty" json:"fix-versions,omitempty"`
	AffectsVersions map[string][]string `toml:"affects-versions,omitempty" json:"affects-versions,omitempty"`
	AssigneeMap     map[string]string   `toml:"assignee,omitempty" json:"assignee,omitempty"`

	// JIRA custom field keys map
	FieldIDs map[fieldKey]string
}

// NewConfig create new config
func NewConfig() *Config {
	config := &Config{}
	config.FlagSet = flag.NewFlagSet("sync-jira", flag.ContinueOnError)
	fs := config.FlagSet

	fs.BoolVar(&config.Version, "V", false, "print version information and exit")

	fs.IntVar(&config.ListenPort, "listen-port", 8080, "port used to listen github webhook")
	fs.StringVar(&config.GithubUsername, "github-username", "", "GitHub username")
	fs.StringVar(&config.GithubPassword, "github-password", "", "GitHub password")
	fs.StringVar(&config.JiraUsername, "jira-username", "", "JIRA username")
	fs.StringVar(&config.JiraPassword, "jira-password", "", "JIRA password")
	fs.StringVar(&config.JiraBaseURL, "jira-baseurl", "", "JIRA endpoint url")

	fs.BoolVar(&config.DoPreSync, "do-presync", true, "Do pre-synchronization")

	fs.BoolVar(&config.UseLastSyncTimeFile, "use-lastsynctimefile", false, "Use last sync time file")

	fs.StringVar(&config.configFile, "config", "./config.toml", "path to config file")
	fs.StringVar(&config.LogLevel, "L", "debug", "log level: debug, info, warn, error, fatal")

	locChina, _ := time.LoadLocation("Asia/Shanghai")
	config.Loc = locChina
	config.GithubIssueSince = time.Now().AddDate(0, -3, 0) // sync since recent 3 months

	return config
}

// Parse parses all config from command-line flags
func (config *Config) Parse(args []string) error {
	perr := config.FlagSet.Parse(args)
	switch perr {
	case nil:
	case flag.ErrHelp:
		os.Exit(0)
	default:
		os.Exit(2)
	}

	if config.Version {
		PrintVersionInfo()
		os.Exit(0)
	}

	// Load config file if specified.
	if config.configFile != "" {
		err := config.configFromFile(config.configFile)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Parse again to replace with command line options.
	err := config.FlagSet.Parse(args)
	if err != nil {
		return errors.Trace(err)
	}

	if len(config.FlagSet.Args()) != 0 {
		return errors.Errorf("'%s' is an invalid flag", config.FlagSet.Arg(0))
	}

	if config.JiraBaseURL == "" {
		return errors.New("JIRA base URL should be given")
	}

	if config.JiraUsername == "" {
		return errors.New("JIRA username should be given")
	}

	if config.JiraPassword == "" {
		return errors.New("JIRA password should be given")
	}

	return nil
}

func (config *Config) configFromFile(configFile string) error {
	_, err := toml.DecodeFile(configFile, config)
	return errors.Trace(err)
}

// Version information.
var (
	BuildTS   = "None"
	GitHash   = "None"
	GitBranch = "None"
)

// PrintVersionInfo prints the version information without log info.
func PrintVersionInfo() {
	fmt.Println("Git Commit Hash:", GitHash)
	fmt.Println("Git Branch:", GitBranch)
	fmt.Println("UTC Build Time: ", BuildTS)
}

// following code is to deal with jira custom fields keys

type fieldKey int

// fieldKey is an enum-like type to represent the customfield ID keys
const (
	gitHubID fieldKey = iota
	gitHubURL
	gitHubNumber
	gitHubLabels
	gitHubStatus
	gitHubReporter
	lastISUpdate
)

type jiraField struct {
	ID          string   `json:"id"`
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Custom      bool     `json:"custom"`
	Orderable   bool     `json:"orderable"`
	Navigable   bool     `json:"navigable"`
	Searchable  bool     `json:"searchable"`
	ClauseNames []string `json:"clauseNames"`
	Schema      struct {
		Type     string `json:"type"`
		System   string `json:"system,omitempty"`
		Items    string `json:"items,omitempty"`
		Custom   string `json:"custom,omitempty"`
		CustomID int    `json:"customId,omitempty"`
	} `json:"schema,omitempty"`
}

// use createmeta or field api, or jira.GetCustomFields
func getJiraFiledIDs(jiraClient *jira.Client) (map[fieldKey]string, error) {
	req, err := jiraClient.NewRequest("GET", "rest/api/2/field", nil)
	if err != nil {
		return map[fieldKey]string{}, err
	}

	jiraFields := new([]jiraField)
	resp, err := jiraClient.Do(req, jiraFields)
	if err != nil {
		err := jira.NewJiraError(resp, err)
		logrus.WithError(err).Error("get jira custom field error")
		return map[fieldKey]string{}, err
	}

	fieldIDs := map[fieldKey]string{
		gitHubID:       "",
		gitHubNumber:   "",
		gitHubLabels:   "",
		gitHubStatus:   "",
		gitHubReporter: "",
		lastISUpdate:   "",
	}

	for _, field := range *jiraFields {
		switch field.Name {
		case "GitHub ID":
			fieldIDs[gitHubID] = fmt.Sprint(field.Schema.CustomID)
		case "GitHub URL":
			fieldIDs[gitHubURL] = fmt.Sprint(field.Schema.CustomID)
		case "GitHub Number":
			fieldIDs[gitHubNumber] = fmt.Sprint(field.Schema.CustomID)
		case "GitHub Labels":
			fieldIDs[gitHubLabels] = fmt.Sprint(field.Schema.CustomID)
		case "GitHub Status":
			fieldIDs[gitHubStatus] = fmt.Sprint(field.Schema.CustomID)
		case "GitHub Reporter":
			fieldIDs[gitHubReporter] = fmt.Sprint(field.Schema.CustomID)
		case "Last Issue-Sync Update":
			fieldIDs[lastISUpdate] = fmt.Sprint(field.Schema.CustomID)
		}
	}

	if fieldIDs[gitHubID] == "" {
		// using errors.notfound
		// return fieldIDs, errors.NotFoundf()
		return fieldIDs, errors.New("Could not find JIRA custom field ID of 'GitHub ID' custom field; check that it is named correctly")
	}

	return fieldIDs, nil
}

// return number string with 'customfield_' prefix, e.g. "customfield_10109"
func (config *Config) getFieldID(key fieldKey) (string, error) {
	val, ok := config.FieldIDs[key]
	if !ok {
		return "", errors.New("fieldKey not exists")
	}
	return fmt.Sprintf("customfield_%s", val), nil
}

// return just number string, e.g. "10109"
func (config *Config) getFieldKey(key fieldKey) (string, error) {
	val, ok := config.FieldIDs[key]
	if !ok {
		return "", errors.New("fieldKey not exists")
	}
	return val, nil
}
