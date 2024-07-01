package main

import (
	"runtime"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/commands"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
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
