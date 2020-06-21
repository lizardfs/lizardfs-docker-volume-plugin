package main

import (
	"context"
	"fmt"
	"github.com/docker/go-plugins-helpers/volume"
	"github.com/ramr/go-reaper"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const socketAddress = "/run/docker/plugins/lizardfs.sock"
const containerVolumePath = "/mnt/docker-volumes"
const hostVolumePath = "/mnt/docker-volumes"
const volumeRoot = "/mnt/lizardfs/"

var host = os.Getenv("HOST")
var port = os.Getenv("PORT")
var remotePath = os.Getenv("REMOTE_PATH")
var mountOptions = os.Getenv("MOUNT_OPTIONS")
var rootVolumeName = os.Getenv("ROOT_VOLUME_NAME")
var connectTimeoutStr = os.Getenv("CONNECT_TIMEOUT")
var connectTimeout = 3000

var mounted = make(map[string][]string)

type lizardfsVolume struct {
	Name string
	Goal int
	Path string
}

type lizardfsDriver struct {
	volumes   map[string]*lizardfsVolume
	statePath string
}

func (l lizardfsDriver) Create(request *volume.CreateRequest) error {
	log.WithField("method", "create").Debugf("%#v", l)
	volumeName := request.Name
	volumePath := fmt.Sprintf("%s%s", volumeRoot, volumeName)
	replicationGoal := request.Options["ReplicationGoal"]

	if volumeName == rootVolumeName {
		log.Warning("tried to create a volume with same name as root volume. Ignoring request.")
	}

	err := os.MkdirAll(volumePath, 760)
	if err != nil {
		return err
	}
	_, err = strconv.Atoi(replicationGoal)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(connectTimeout)*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, "lizardfs", "setgoal", "-r", replicationGoal, volumePath)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: 1}
		err = cmd.Start()
		if err != nil {
			return err
		}
		err = cmd.Wait()
		if err != nil {
			log.Error(err)
		}
	}

	return nil
}

func (l lizardfsDriver) List() (*volume.ListResponse, error) {
	log.WithField("method", "list").Debugf("")
	var vols []*volume.Volume
	directories, err := ioutil.ReadDir(volumeRoot)
	for _, directory := range directories {
		if len(mounted[directory.Name()]) == 0 {
			vols = append(vols, &volume.Volume{Name: directory.Name()})
		} else {
			vols = append(vols, &volume.Volume{Name: directory.Name(), Mountpoint: path.Join(hostVolumePath, directory.Name())})
		}
	}
	if err != nil {
		return nil, err
	}
	if rootVolumeName != "" {
		if len(mounted[rootVolumeName]) == 0 {
			vols = append(vols, &volume.Volume{Name: rootVolumeName})
		} else {
			vols = append(vols, &volume.Volume{Name: rootVolumeName, Mountpoint: path.Join(hostVolumePath, rootVolumeName)})
		}
	}
	return &volume.ListResponse{Volumes: vols}, nil
}

func (l lizardfsDriver) Get(request *volume.GetRequest) (*volume.GetResponse, error) {
	log.WithField("method", "get").Debugf("")
	volumeName := request.Name
	volumePath := volumeRoot
	if volumeName != rootVolumeName {
		volumePath = fmt.Sprintf("%s%s", volumeRoot, volumeName)
	}
	if _, err := os.Stat(volumePath); os.IsNotExist(err) {
		return nil, err
	}
	return &volume.GetResponse{Volume: &volume.Volume{Name: volumeName, Mountpoint: volumePath}}, nil
}

func (l lizardfsDriver) Remove(request *volume.RemoveRequest) error {
	log.WithField("method", "remove").Debugf("")
	volumeName := request.Name
	volumePath := fmt.Sprintf("%s%s", volumeRoot, volumeName)

	if volumeName == rootVolumeName {
		return fmt.Errorf("can't remove root volume %s", rootVolumeName)
	}

	err := os.RemoveAll(volumePath)
	return err
}

func (l lizardfsDriver) Path(request *volume.PathRequest) (*volume.PathResponse, error) {
	log.WithField("method", "path").Debugf("")
	var volumeName = request.Name
	var hostMountpoint = path.Join(hostVolumePath, volumeName)

	if len(mounted[volumeName]) == 0 {
		return &volume.PathResponse{Mountpoint: hostMountpoint}, nil
	}
	return &volume.PathResponse{}, nil
}

func (l lizardfsDriver) Mount(request *volume.MountRequest) (*volume.MountResponse, error) {
	log.WithField("method", "mount").Debugf("")
	var volumeName = request.Name
	var mountID = request.ID
	var containerMountpoint = path.Join(containerVolumePath, volumeName)
	var hostMountpoint = path.Join(hostVolumePath, volumeName)

	if len(mounted[volumeName]) == 0 {
		err := os.MkdirAll(containerMountpoint, 760)
		if err != nil && err != os.ErrExist {
			return nil, err
		}

		mountRemotePath := remotePath

		if volumeName != rootVolumeName {
			mountRemotePath = path.Join(remotePath, volumeName)
		}

		params := []string{containerMountpoint, "-H", host, "-P", port, "-S", mountRemotePath}
		if mountOptions != "" {
			params = append(params, strings.Split(mountOptions, " ")...)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(connectTimeout)*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, "lfsmount", params...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: 1}
		err = cmd.Start()
		if err != nil {
			return nil, err
		}
		err = cmd.Wait()
		if err != nil {
			log.Error(err)
		}
		mounted[volumeName] = append(mounted[volumeName], mountID)
		return &volume.MountResponse{Mountpoint: hostMountpoint}, nil
	} else {
		return &volume.MountResponse{Mountpoint: hostMountpoint}, nil
	}
}

func indexOf(word string, data []string) int {
	for k, v := range data {
		if word == v {
			return k
		}
	}
	return -1
}

func (l lizardfsDriver) Unmount(request *volume.UnmountRequest) error {
	log.WithField("method", "unmount").Debugf("")
	var volumeName = request.Name
	var mountID = request.ID
	var containerMountpoint = path.Join(containerVolumePath, volumeName)

	index := indexOf(mountID, mounted[volumeName])

	if index > -1 {
		mounted[volumeName] = append(mounted[volumeName][:index], mounted[volumeName][index+1:]...)
	}
	if len(mounted[volumeName]) == 0 {
		output, err := exec.Command("umount", containerMountpoint).CombinedOutput()
		if err != nil {
			log.Error(string(output))
			return err
		}
		log.Debug(string(output))
		return nil
	}
	return nil
}

func (l lizardfsDriver) Capabilities() *volume.CapabilitiesResponse {
	log.WithField("method", "capabilities").Debugf("")
	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "global"}}
}

func newLizardfsDriver(root string) (*lizardfsDriver, error) {
	log.WithField("method", "new driver").Debug(root)

	d := &lizardfsDriver{
		volumes: map[string]*lizardfsVolume{},
	}

	return d, nil
}

func initClient() {
	log.WithField("host", host).WithField("port", port).WithField("remote path", remotePath).Info("initializing client")
	err := os.MkdirAll(volumeRoot, 760)
	if err != nil {
		log.Error(err)
	}
	params := []string{volumeRoot, "-H", host, "-P", port, "-S", remotePath}
	if mountOptions != "" {
		params = append(params, strings.Split(mountOptions, " ")...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(connectTimeout)*time.Millisecond)
	defer cancel()

	output, err := exec.CommandContext(ctx, "lfsmount", params...).CombinedOutput()
	if err != nil {
		log.Error(string(output))
		log.Fatal(err)
	}
	log.Debug(string(output))
}

func startReaperWorker() {
	// See related issue in go-reaper https://github.com/ramr/go-reaper/issues/11
	if _, hasReaper := os.LookupEnv("REAPER"); !hasReaper {
		go reaper.Reap()

		args := append(os.Args, "#worker")

		pwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		workerEnv := []string{fmt.Sprintf("REAPER=%d", os.Getpid())}

		var wstatus syscall.WaitStatus
		pattrs := &syscall.ProcAttr{
			Dir:   pwd,
			Env:   append(os.Environ(), workerEnv...),
			Sys:   &syscall.SysProcAttr{Setsid: true},
			Files: []uintptr{0, 1, 2},
		}
		workerPid, _ := syscall.ForkExec(args[0], args, pattrs)
		_, err = syscall.Wait4(workerPid, &wstatus, 0, nil)
		for syscall.EINTR == err {
			_, err = syscall.Wait4(workerPid, &wstatus, 0, nil)
		}
	}
}

func main() {
	logLevel, err := log.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(logLevel)
	}
	log.Debugf("log level set to %s", log.GetLevel())
	startReaperWorker()
	connectTimeout, err = strconv.Atoi(connectTimeoutStr)
	if err != nil {
		log.Errorf("failed to parse timeout with error %v. Assuming default %v", err, connectTimeout)
	}
	initClient()

	d, err := newLizardfsDriver("/mnt")
	if err != nil {
		log.Fatal(err)
	}
	h := volume.NewHandler(d)
	log.Infof("listening on %s", socketAddress)
	log.Error(h.ServeUnix(socketAddress, 0))
}
