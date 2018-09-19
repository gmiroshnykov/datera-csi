package client

import (
	"context"
	"fmt"
	"strings"

	co "github.com/Datera/datera-csi/pkg/common"
	dsdk "github.com/Datera/go-sdk/pkg/dsdk"
)

type VolOpts struct {
	Size            int
	Replica         int
	Template        string
	FsType          string
	FsArgs          string
	PlacementMode   string
	CloneSrc        string
	CloneVolSrc     string
	CloneSnapSrc    string
	IpPool          string
	RoundRobin      bool
	DeleteOnUnmount bool

	// QoS IOPS
	WriteIopsMax int
	ReadIopsMax  int
	TotalIopsMax int

	// QoS Bandwidth
	WriteBandwidthMax int
	ReadBandwidthMax  int
	TotalBandwidthMax int

	// Dynamic QoS
	IopsPerGb      int
	BandwidthPerGb int
}

type Volume struct {
	ctxt           context.Context
	Ai             *dsdk.AppInstance
	Name           string
	AdminState     string
	RepairPriority string
	Template       string

	TargetOpState string
	Ips           []string
	Iqn           string
	Initiators    []string

	Replicas      int
	PlacementMode string
	Size          int

	// QoS in map form, mostly for logging
	QoS map[string]int

	// Direct Access QoS Iops
	WriteIopsMax int
	ReadIopsMax  int
	TotalIopsMax int
	// Direct Access QoS Iops
	WriteBandwidthMax int
	ReadBandwidthMax  int
	TotalBandwidthMax int

	DevicePath string
	MountPath  string
	FsType     string
	FsArgs     []string
}

type VolMetadata map[string]string

func AiToClientVol(ctx context.Context, ai *dsdk.AppInstance, qos bool, client *DateraClient) (*Volume, error) {
	ctxt := context.WithValue(ctx, co.ReqName, "AiToClientVol")
	if ai == nil {
		return nil, fmt.Errorf("Cannot construct a Client Volume from a nil AppInstance")
	}
	si := ai.StorageInstances[0]
	v := si.Volumes[0]
	inits := []string{}
	for _, init := range si.AclPolicy.Initiators {
		inits = append(inits, init.Name)
	}
	var pp map[string]int
	if qos && client != nil {
		resp, apierr, err := v.PerformancePolicy.Get(&dsdk.PerformancePolicyGetRequest{
			Ctxt: ctxt,
		})
		if err != nil {
			co.Error(ctxt, err)
			return nil, err
		} else if apierr != nil {
			co.Errorf(ctxt, "%s, %s", dsdk.Pretty(apierr), err)
			return nil, fmt.Errorf("ApiError: %#v", *apierr)
		}
		pp = map[string]int{
			"read_iops_max":       resp.ReadIopsMax,
			"write_iops_max":      resp.WriteIopsMax,
			"total_iops_max":      resp.TotalIopsMax,
			"read_bandwidth_max":  resp.ReadBandwidthMax,
			"write_bandwidth_max": resp.WriteBandwidthMax,
			"total_bandwidth_max": resp.TotalBandwidthMax,
		}
	}

	return &Volume{
		ctxt:           ctxt,
		Ai:             ai,
		Name:           ai.Name,
		AdminState:     ai.AdminState,
		RepairPriority: ai.RepairPriority,
		Template:       ai.AppTemplate.Name,

		TargetOpState: si.OpState,
		Ips:           si.Access.Ips,
		Iqn:           si.Access.Iqn,
		Initiators:    inits,

		Replicas:      v.ReplicaCount,
		PlacementMode: v.PlacementMode,
		Size:          v.Size,

		QoS:               pp,
		ReadIopsMax:       pp["read_iops_max"],
		WriteIopsMax:      pp["write_iops_max"],
		TotalIopsMax:      pp["total_iops_max"],
		ReadBandwidthMax:  pp["read_bandwidth_max"],
		WriteBandwidthMax: pp["write_bandwidth_max"],
		TotalBandwidthMax: pp["total_bandwidth_max"],
	}, nil
}

func (r DateraClient) GetVolume(name string, qos bool) (*Volume, error) {
	ctxt := context.WithValue(r.ctxt, co.ReqName, "GetVolume")
	co.Debugf(ctxt, "GetVolume invoked for %s", name)
	newAi, apierr, err := r.sdk.AppInstances.Get(&dsdk.AppInstancesGetRequest{
		Ctxt: ctxt,
		Id:   name,
	})
	if err != nil {
		return nil, err
	}
	if apierr != nil {
		return nil, fmt.Errorf("%s", apierr.Name)
	}
	v, err := AiToClientVol(ctxt, newAi, qos, &r)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (r DateraClient) CreateVolume(name string, volOpts *VolOpts, qos bool) (*Volume, error) {
	ctxt := context.WithValue(r.ctxt, co.ReqName, "CreateVolume")
	co.Debugf(ctxt, "CreateVolume invoked for %s, volOpts: %#v", name, volOpts)
	var ai dsdk.AppInstancesCreateRequest
	if volOpts.Template != "" {
		// From Template
		template := strings.Trim(volOpts.Template, "/")
		co.Debugf(ctxt, "Creating AppInstance with template: %s", template)
		at := &dsdk.AppTemplate{
			Path: "/app_templates/" + template,
		}
		ai = dsdk.AppInstancesCreateRequest{
			Ctxt:        ctxt,
			Name:        name,
			AppTemplate: at,
		}
	} else if volOpts.CloneVolSrc != "" {
		// Clone Volume
		c := &dsdk.Volume{Path: volOpts.CloneVolSrc}
		co.Debugf(ctxt, "Creating AppInstance from Volume clone: %s", volOpts.CloneVolSrc)
		ai = dsdk.AppInstancesCreateRequest{
			Ctxt:           ctxt,
			Name:           name,
			CloneVolumeSrc: c,
		}
	} else if volOpts.CloneSnapSrc != "" {
		// Clone Snapshot
		c := &dsdk.Snapshot{Path: volOpts.CloneSnapSrc}
		co.Debugf(ctxt, "Creating AppInstance from Snapshot clone: %s", volOpts.CloneSrc)
		ai = dsdk.AppInstancesCreateRequest{
			Ctxt:             ctxt,
			Name:             name,
			CloneSnapshotSrc: c,
		}
	} else {
		// Vanilla Volume Create
		vol := &dsdk.Volume{
			Name:          "volume-1",
			Size:          int(volOpts.Size),
			PlacementMode: volOpts.PlacementMode,
			ReplicaCount:  int(volOpts.Replica),
		}
		si := &dsdk.StorageInstance{
			Name:    "storage-1",
			Volumes: []*dsdk.Volume{vol},
		}
		ai = dsdk.AppInstancesCreateRequest{
			Ctxt:             ctxt,
			Name:             name,
			StorageInstances: []*dsdk.StorageInstance{si},
		}
	}
	newAi, apierr, err := r.sdk.AppInstances.Create(&ai)
	if err != nil {
		co.Error(ctxt, err)
		return nil, err
	} else if apierr != nil {
		co.Errorf(ctxt, "%s, %s", dsdk.Pretty(apierr), err)
		return nil, fmt.Errorf("ApiError: %#v", *apierr)
	}
	v, err := AiToClientVol(ctxt, newAi, false, &r)
	if qos {
		if err = v.SetPerformancePolicy(volOpts); err != nil {
			return nil, err
		}
	}
	if err != nil {
		co.Error(ctxt, err)
		return nil, err
	}
	return v, nil
}

func (r DateraClient) DeleteVolume(name string, force bool) error {
	ctxt := context.WithValue(r.ctxt, co.ReqName, "DeleteVolume")
	co.Debugf(ctxt, "DeleteVolume invoked for %s", name)
	ai, apierr, err := r.sdk.AppInstances.Get(&dsdk.AppInstancesGetRequest{
		Ctxt: ctxt,
		Id:   name,
	})
	v, err := AiToClientVol(ctxt, ai, false, &r)
	if err != nil {
		co.Error(ctxt, err)
		return err
	} else if apierr != nil {
		co.Errorf(ctxt, "%s, %s", dsdk.Pretty(apierr), err)
		return fmt.Errorf("ApiError: %#v", *apierr)
	}
	return v.Delete(force)
}

func (r *Volume) Delete(force bool) error {
	ctxt := context.WithValue(r.ctxt, co.ReqName, "Delete")
	co.Debugf(ctxt, "Volume Delete invoked for %s", r.Name)
	_, apierr, err := r.Ai.Set(&dsdk.AppInstanceSetRequest{
		Ctxt:       ctxt,
		AdminState: "offline",
		Force:      force,
	})
	if err != nil {
		co.Error(ctxt, err)
		return err
	} else if apierr != nil {
		co.Errorf(ctxt, "%s, %s", dsdk.Pretty(apierr), err)
		return fmt.Errorf("ApiError: %#v", *apierr)
	}
	_, apierr, err = r.Ai.Delete(&dsdk.AppInstanceDeleteRequest{
		Ctxt:  ctxt,
		Force: force,
	})
	if err != nil {
		co.Error(ctxt, err)
		return err
	} else if apierr != nil {
		co.Errorf(ctxt, "%s, %s", dsdk.Pretty(apierr), err)
		return fmt.Errorf("ApiError: %#v", *apierr)
	}
	return nil
}

func (r DateraClient) ListVolumes(maxEntries int, startToken int) ([]*Volume, error) {
	ctxt := context.WithValue(r.ctxt, co.ReqName, "ListVolumes")
	co.Debug(ctxt, "ListVolumes invoked\n")
	params := dsdk.ListParams{
		Limit:  maxEntries,
		Offset: startToken,
	}
	resp, apierr, err := r.sdk.AppInstances.List(&dsdk.AppInstancesListRequest{
		Ctxt:   ctxt,
		Params: params,
	})
	if err != nil {
		co.Error(ctxt, err)
		return nil, err
	} else if apierr != nil {
		co.Errorf(ctxt, "%s, %s", dsdk.Pretty(apierr), err)
		return nil, fmt.Errorf("ApiError: %#v", *apierr)
	}
	vols := []*Volume{}
	for _, ai := range resp {
		v, err := AiToClientVol(ctxt, ai, false, &r)
		if err != nil {
			co.Error(ctxt, err)
			continue
		}
		vols = append(vols, v)
	}
	return vols, nil
}

func (r *Volume) SetPerformancePolicy(volOpts *VolOpts) error {
	ctxt := context.WithValue(r.ctxt, co.ReqName, "SetPerformancePolicy")
	co.Debugf(ctxt, "SetPerformancePolicy invoked for %s, volOpts: %#v", r.Name, volOpts)
	ai := r.Ai
	pp := dsdk.PerformancePolicyCreateRequest{
		Ctxt:              ctxt,
		ReadIopsMax:       int(volOpts.ReadIopsMax),
		WriteIopsMax:      int(volOpts.WriteIopsMax),
		TotalIopsMax:      int(volOpts.TotalIopsMax),
		ReadBandwidthMax:  int(volOpts.ReadBandwidthMax),
		WriteBandwidthMax: int(volOpts.WriteBandwidthMax),
		TotalBandwidthMax: int(volOpts.TotalBandwidthMax),
	}
	resp, apierr, err := ai.StorageInstances[0].Volumes[0].PerformancePolicy.Create(&pp)
	if err != nil {
		co.Error(ctxt, err)
		return err
	} else if apierr != nil {
		co.Errorf(ctxt, "%s, %s", dsdk.Pretty(apierr), err)
		return fmt.Errorf("ApiError: %#v", *apierr)
	}
	r.QoS = map[string]int{
		"read_iops_max":       resp.ReadIopsMax,
		"write_iops_max":      resp.WriteIopsMax,
		"total_iops_max":      resp.TotalIopsMax,
		"read_bandwidth_max":  resp.ReadBandwidthMax,
		"write_bandwidth_max": resp.WriteBandwidthMax,
		"total_bandwidth_max": resp.TotalBandwidthMax,
	}
	r.ReadIopsMax = resp.ReadIopsMax
	r.WriteIopsMax = resp.WriteIopsMax
	r.TotalIopsMax = resp.TotalIopsMax
	r.ReadBandwidthMax = resp.ReadBandwidthMax
	r.WriteBandwidthMax = resp.WriteBandwidthMax
	r.TotalBandwidthMax = resp.TotalBandwidthMax
	return nil
}

func (r *Volume) GetMetadata() (*VolMetadata, error) {
	ctxt := context.WithValue(r.ctxt, co.ReqName, "GetMetadata")
	co.Debugf(ctxt, "GetMetadata invoked for %s", r.Name)
	resp, apierr, err := r.Ai.GetMetadata(&dsdk.AppInstanceMetadataGetRequest{
		Ctxt: ctxt,
	})
	if err != nil {
		co.Error(ctxt, err)
		return nil, err
	} else if apierr != nil {
		co.Errorf(ctxt, "%s, %s", dsdk.Pretty(apierr), err)
		return nil, fmt.Errorf("ApiError: %#v", *apierr)
	}
	result := VolMetadata(*resp)
	return &result, nil
}

func (r *Volume) SetMetadata(metadata *VolMetadata) (*VolMetadata, error) {
	ctxt := context.WithValue(r.ctxt, co.ReqName, "GetMetadata")
	co.Debugf(ctxt, "GetMetadata invoked for %s", r.Name)
	resp, apierr, err := r.Ai.SetMetadata(&dsdk.AppInstanceMetadataSetRequest{
		Ctxt:     ctxt,
		Metadata: *metadata,
	})
	if err != nil {
		co.Error(ctxt, err)
		return nil, err
	} else if apierr != nil {
		co.Errorf(ctxt, "%s, %s", dsdk.Pretty(apierr), err)
		return nil, fmt.Errorf("ApiError: %#v", *apierr)
	}
	result := VolMetadata(*resp)
	return &result, nil
}

func (r *Volume) GetUsage() (int, int, int) {
	ctxt := context.WithValue(r.ctxt, co.ReqName, "GetUsage")
	co.Debugf(ctxt, "GetUsage invoked for %s", r.Name)
	v := r.Ai.StorageInstances[0].Volumes[0]
	size := v.Size
	used := v.CapacityInUse
	avail := size - used
	return size, used, avail
}
