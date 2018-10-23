package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	co "github.com/Datera/datera-csi/pkg/common"
	pb "github.com/Datera/datera-csi/pkg/iscsi-rpc"
	"google.golang.org/grpc"
)

const (
	address  = "unix:///iscsi-socket/iscsi.sock"
	etcIscsi = "/etc/iscsi/initiatorname.iscsi"
)

var (
	addr = flag.String("addr", address, "Address to send on")
	args = flag.String("args", "", "Arguments including iscsiadm prefix")
)

func setupInitName(ctxt context.Context, c pb.IscsiadmClient) {
	// co.Debugf(ctxt, "Checking for file: %s\n", addr)
	if _, err := os.Stat(etcIscsi); os.IsNotExist(err) {
		co.Debugf(ctxt, "Creating directories: %s\n", addr)
		err = os.MkdirAll(etcIscsi, os.ModePerm)
		if err != nil {
			co.Fatal(ctxt, err)
		}
		resp, err := c.GetInitiatorName(ctxt, &pb.GetInitiatorNameRequest{})
		if err != nil {
			co.Fatal(ctxt, err)
		}
		name := fmt.Sprintf("InitiatorName=%s", resp.Name)
		err = ioutil.WriteFile(etcIscsi, []byte(name), 0644)
		if err != nil {
			co.Fatal(ctxt, err)
		}
	}
}

func main() {
	flag.Parse()
	// Set up a connection to the server.
	conn, err := grpc.Dial(*addr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewIscsiadmClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	ctx = co.WithCtxt(ctx, "iscsi-send")
	defer cancel()
	setupInitName(ctx, c)
	r, err := c.SendArgs(ctx, &pb.SendArgsRequest{Args: *args})
	if err != nil {
		co.Fatalf(ctx, "Could not send args: %v", err)
	}
	co.Debugf(ctx, "Iscsiadm Result: %s", r.Result)
}