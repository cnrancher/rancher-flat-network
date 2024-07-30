package logger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/sirupsen/logrus"
)

const (
	loggingFlagFile = "/etc/rancher/flat-network/cni-loglevel.conf"
	logDir          = "/var/log/rancher-flat-network/"
	logFileFormat   = "/var/log/rancher-flat-network/%s.log"
	arpNotifyPolicy = "arp_notify"
	arpintPolicy    = "arping"
)

// Setup logrus loglevel and output file.
// Output logfile to 'logFileFormat' only when 'loggingFlagFile' exists.
func Setup() error {
	logrus.SetOutput(io.Discard) // Discard log output by default
	logrus.SetFormatter(&nested.Formatter{
		HideKeys:        false,
		TimestampFormat: time.DateTime,
		NoColors:        true,
	})
	logrus.SetLevel(logrus.InfoLevel)

	if _, err := os.Stat(loggingFlagFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to stat %v: %w", loggingFlagFile, err)
	}

	data, err := os.ReadFile(loggingFlagFile)
	if err != nil {
		return fmt.Errorf("failed to read %v: %w", loggingFlagFile, err)
	}
	content := strings.TrimSpace(string(data))
	level, err := logrus.ParseLevel(content)
	if err != nil {
		return fmt.Errorf("failed to parse loglevel %q: %w", content, err)
	}
	logrus.SetLevel(level)

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to mkdir %q: %w", logDir, err)
	}
	// Separate log file in date
	logFile := fmt.Sprintf(logFileFormat, time.Now().Format(time.DateOnly))
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", logFile, err)
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("failed to seek log file %q: %w", logFile, err)
	}
	logrus.SetOutput(f)

	logrus.Infof("---------------------------------------")
	logrus.Debugf("CNI run at %v", time.Now().String())
	logrus.Infof("set log level to: %v", level.String())

	return nil
}
