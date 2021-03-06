package main

import (
	"context"
	"flag"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	dc "github.com/Datera/datera-csi/pkg/client"
	co "github.com/Datera/datera-csi/pkg/common"
	pb "github.com/Datera/datera-csi/pkg/iscsi-rpc"
	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	status "google.golang.org/grpc/status"
)

const (
	address = "unix:///iscsi-socket/iscsi.sock"
)

var (
	// Necessary to prevent UDC arguments from showing up
	cli = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	addr = cli.String("addr", address, "Address to send on")
)

// server is used to implement helloworld.GreeterServer.
type server struct{}

func (s *server) SendArgs(ctx context.Context, in *pb.SendArgsRequest) (*pb.SendArgsReply, error) {
	ctxt := co.WithCtxt(ctx, "iscsi-recv SendArgs", "")
	co.Debugf(ctxt, "Recieved message, %#v", in)
	cmd := strings.Split(in.Args, " ")
	result, err := co.RunCmd(ctxt, cmd...)
	if err != nil {
		return nil, status.Errorf(codes.Unknown, err.Error())
	}
	return &pb.SendArgsReply{Result: result}, nil
}

func (s *server) GetInitiatorName(ctx context.Context, in *pb.GetInitiatorNameRequest) (*pb.GetInitiatorNameReply, error) {
	ctxt := co.WithCtxt(ctx, "iscsi-recv GetInitiatorName", "")
	iqn, err := dc.GetClientIqn(ctxt)
	if err != nil {
		return nil, status.Errorf(codes.Unknown, err.Error())
	}
	return &pb.GetInitiatorNameReply{Name: iqn}, nil
}

func main() {
	cli.Parse(os.Args[1:])

	ctxt := co.WithCtxt(context.Background(), "iscsi-recv", "")
	u, err := url.Parse(*addr)
	if err != nil {
		co.Fatal(ctxt, err)
	}
	addr := path.Join(u.Host, filepath.FromSlash(u.Path))
	if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
		co.Fatalf(ctxt, "Failed to remove unix domain socket file: %s", addr)
	}
	lis, err := net.Listen("unix", addr)
	if err != nil {
		co.Fatalf(ctxt, "failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterIscsiadmServer(s, &server{})
	// Register reflection service on gRPC server.
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		co.Fatalf(ctxt, "failed to serve: %v", err)
	}
}
