package main

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/juju/errors"
	logrus "github.com/sirupsen/logrus"
)

func main() {

	// log settings
	logrus.SetOutput(os.Stdout)

	// set up Configuration
	Config := NewConfig()
	if err := Config.Parse(os.Args[1:]); err != nil {
		logrus.Fatalf("verifying flags error %s", errors.ErrorStack(err))
	}
	logrusSetLevelByString(Config.LogLevel)
	PrintVersionInfo()
	logrus.Debugf("\nConfig is: %+v\n", Config)

	// create new server
	server, err := newServer(Config)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating server")
	}

	// compare and sync issues to JIRA before the server start to listen
	// !! there is corner case when doing this, new webhook events arrive
	if server.Config.DoPreSync {
		start := time.Now()
		logrus.Info("start compare and sync issues from GitHub to JIRA")
		l := logrus.WithFields(logrus.Fields{
			"event-type": "compareSyncIssues",
		},
		)
		err = server.compareSyncIssues(l)
		if err != nil {
			logrus.WithError(err).Fatal("compare sync issuues failed")
		}
		logrus.Info("success compare and sync issues from GitHub to JIRA")
		logrus.Infof("which took about %v", time.Since(start))
	} else {
		logrus.Info("bypass presync")
	}

	// start server to listen to github webhook
	http.Handle("/", server)
	logrus.Fatal(http.ListenAndServe(":"+strconv.Itoa(server.Config.ListenPort), nil))
}
