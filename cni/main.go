package main

import (
	"fmt"
	"runtime"

	"github.com/cnrancher/rancher-flat-network/pkg/cni/commands"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
)

var (
	about string
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()

	about = fmt.Sprintf("rancher-flat-network-cni %v", utils.Version)
	if utils.GitCommit != "" {
		about += " - " + utils.GitCommit
	}
}

func main() {
	versions := version.PluginSupports("0.1.0", "0.2.0", "0.3.0", "0.3.1", "0.4.0", "1.0.0")
	funcs := skel.CNIFuncs{
		Add:   commands.Add,
		Del:   commands.Del,
		Check: commands.Check,
	}
	skel.PluginMainFuncs(funcs, versions, about)
}
