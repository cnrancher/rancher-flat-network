package main

import (
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/commands"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/sirupsen/logrus"
)

const (
	loggingFlagFile = "/cni-loglevel.conf"
	defaultLogFile  = "/var/log/rancher-flat-network-cni.log"
	arpNotifyPolicy = "arp_notify"
	arpintPolicy    = "arping"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
	logrus.SetLevel(logrus.InfoLevel)
	f, err := os.OpenFile(defaultLogFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		logrus.Errorf("failed to open %q: %v", defaultLogFile, err)
	} else {
		logrus.SetOutput(f)
		logrus.Debugf("CNI run at %v", time.Now().String())
	}

	if _, err := os.Stat(loggingFlagFile); err == nil {
		if content, err := os.ReadFile(loggingFlagFile); err == nil {
			content := strings.TrimSpace(string(content))
			level, err := logrus.ParseLevel(content)
			if err != nil {
				logrus.Errorf("failed to parse loglevel %q: %v", content, err)
			} else {
				logrus.SetLevel(level)
				logrus.Debugf("set log level: %v", level.String())
			}
		}
	}
}

func main() {
	versions := version.PluginSupports("0.1.0", "0.2.0", "0.3.0", "0.3.1", "0.4.0", "1.0.0")
	funcs := skel.CNIFuncs{
		Add:   commands.Add,
		Del:   commands.Del,
		Check: commands.Check,
	}
	skel.PluginMainFuncs(funcs, versions, "rancher-flat-network-cni")
}
