/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nfs

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
)

type nodeServer struct {
	Driver        *nfsDriver
	mounter       mount.Interface
	createFolders bool
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	notMnt, err := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mo := req.GetVolumeCapability().GetMount().GetMountFlags()
	if req.GetReadonly() {
		mo = append(mo, "ro")
	}

	s := req.GetVolumeContext()["server"]
	ep := req.GetVolumeContext()["share"]
	ns.createFolders, err = strconv.ParseBool(req.GetVolumeContext()["createFolders"])
	if err != nil {
		ns.createFolders = false
	}
	source := fmt.Sprintf("%s:%s", s, ep)

	if ns.createFolders {
		err = ns.CreateFolders(source, mo)
		if err != nil {
			return nil, err
		}
	}
	err = ns.mounter.Mount(source, targetPath, "nfs", mo)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	notMnt, err := ns.mounter.IsLikelyNotMountPoint(targetPath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "Targetpath not found")
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if notMnt {
		return nil, status.Error(codes.NotFound, "Volume not mounted")
	}

	err = mount.CleanupMountPoint(req.GetTargetPath(), ns.mounter, false)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	glog.V(5).Infof("Using default NodeGetInfo")

	return &csi.NodeGetInfoResponse{
		NodeId: ns.Driver.nodeID,
	}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	glog.V(5).Infof("Using default NodeGetCapabilities")

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
		},
	}, nil
}

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, in *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) CreateFolders(share string, mo []string) (err error) {
	loggeer := logrus.New()
	loggeer.Level = logrus.WarnLevel
	if os.Getenv("DEBUG") == "1" {
		loggeer.Level = logrus.DebugLevel
	}

	targetPath, err := ioutil.TempDir("/tmp", "FailoCreator")
	loggeer.Debug(targetPath, err)
	if err != nil {
		log.Fatal(err)
	}

	permissions := os.ModePerm
	loggeer.Debug(permissions)

	splitted1 := strings.Split(share, ":")
	loggeer.Debug(splitted1)

	splitted2 := strings.Split(splitted1[1], "/")[1:]
	loggeer.Debug(splitted2)

	creationMount := fmt.Sprintf("%s:/%s", splitted1[0], splitted2[0])
	loggeer.Debug(creationMount)

	pathToCreate := strings.Join(splitted2[1:], "/")
	loggeer.Debug(pathToCreate)

	err = ns.mounter.Mount(creationMount, targetPath, "nfs", mo)
	loggeer.Debug("mounteing", err)
	if err != nil {
		return
	}

	err = os.MkdirAll(path.Join(targetPath, pathToCreate), permissions)
	loggeer.Debug("Makedirs", err)
	if err != nil {
		return
	}

	for tryings := 0; tryings < 5; tryings++ {
		err = mount.CleanupMountPoint(targetPath, ns.mounter, false)
		loggeer.Debug("Unmounting", tryings, err)
		if err == nil {
			break
		}
		time.Sleep(2)
	}

	err = os.RemoveAll(targetPath)
	loggeer.Debug(err)

	return
}
