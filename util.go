package main

import (
	"io/ioutil"
	"strings"
	"time"

	logrus "github.com/sirupsen/logrus"
)

func logrusSetLevelByString(logLevel string) {
	switch strings.ToLower(logLevel) {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	case "fatal":
		logrus.SetLevel(logrus.FatalLevel)
	default:
		logrus.SetLevel(logrus.DebugLevel)
	}
}

// read lastSyncTime from file and sync from the day before lastSyncTime, if not, sync all issues in last three months
func (s *Server) readLastSyncTime(l *logrus.Entry) {
	if s.Config.UseLastSyncTimeFile {
		b, err := ioutil.ReadFile(lastSyncTimeFileName)
		if err != nil {
			l.WithError(err).Debugf("read %s, sync all issues in last three months", lastSyncTimeFileName)
		} else {
			lastSyncTime, err := time.Parse(time.RFC3339, string(b[:]))
			if err != nil {
				l.WithError(err).Info("parse time error")
			} else {
				l.Debug("lastSyncTime : ", lastSyncTime)
				s.Config.GithubIssueSince = lastSyncTime.AddDate(0, 0, -1)
			}
		}
	}
}

// write lastSyncTime to file, which can be read next time
func (s *Server) writeLastSyncTime(l *logrus.Entry) {
	b := []byte(time.Now().Format(time.RFC3339))
	err := ioutil.WriteFile(lastSyncTimeFileName, b, 0644)
	if err != nil {
		l.WithError(err).Debugf("write \"%s\" error", lastSyncTimeFileName)
	}
	l.Debugf("write lastSyncTime into \"%s\"", lastSyncTimeFileName)
}

// shrinkString shrink too long string into maxLength
func shrinkString(str string, maxLength int) string {
	if len(str) > maxLength {
		return str[:maxLength] + "\n\nNotice: The entered text is too long. It exceeds the allowed limit of 30,000 characters."
	}
	return str
}
