package driver

import (
	"testing"

	// csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	sanity "github.com/kubernetes-csi/csi-test/pkg/sanity"

	udc "github.com/Datera/go-udc/pkg/udc"
)

const (
	Endpoint = "unix:///tmp/test-csi.sock"
)

func getDriver(t *testing.T) *Driver {
	conf, err := udc.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDateraDriver(Endpoint, conf)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDriver(t *testing.T) {
	go func() {
		d := getDriver(t)
		if err := d.Run(); err != nil {
			t.Fatal(err)
		}
	}()
	sc := &sanity.Config{
		TargetPath:  "/mnt/my-publish",
		StagingPath: "/mnt/my-staging",
		Address:     Endpoint,
	}
	sanity.Test(t, sc)
}
