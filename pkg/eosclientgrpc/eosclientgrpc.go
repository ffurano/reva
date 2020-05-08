// Copyright 2018-2020 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package eosclientgrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	gouser "os/user"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/cs3org/reva/pkg/appctx"
	erpc "github.com/cs3org/reva/pkg/eosclientgrpc/eos_grpc"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/acl"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"

	"github.com/cs3org/reva/pkg/logger"
)

const (
	rootUser      = "root"
	versionPrefix = ".sys.v#."
)

// AttrType is the type of extended attribute,
// either system (sys) or user (user).
type AttrType uint32

const (
	// SystemAttr is the system extended attribute.
	SystemAttr AttrType = iota
	// UserAttr is the user extended attribute.
	UserAttr
)

func (at AttrType) String() string {
	switch at {
	case SystemAttr:
		return "sys"
	case UserAttr:
		return "user"
	default:
		return "invalid"
	}
}

// Attribute represents an EOS extended attribute.
type Attribute struct {
	Type     AttrType
	Key, Val string
}

func (a *Attribute) serialize() string {
	return fmt.Sprintf("%s.%s=%s", a.Type, a.Key, a.Val)
}

func (a *Attribute) isValid() bool {
	// validate that an attribute is correct.
	if (a.Type != SystemAttr && a.Type != UserAttr) || a.Key == "" {
		return false
	}
	return true
}

// Options to configure the Client.
type Options struct {

	// ForceSingleUserMode forces all connections to use only one user.
	// This is the case when access to EOS is done from FUSE under apache or www-data.
	ForceSingleUserMode bool

	// UseKeyTabAuth changes will authenticate requests by using an EOS keytab.
	UseKeytab bool

	// SingleUsername is the username to use when connecting to EOS.
	// Defaults to apache
	SingleUsername string

	// Location of the eos binary.
	// Default is /usr/bin/eos.
	EosBinary string

	// Location of the xrdcopy binary.
	// Default is /usr/bin/xrdcopy.
	XrdcopyBinary string

	// URL of the EOS MGM.
	// Default is root://eos-example.org
	URL string

	// URI of the EOS MGM grpc server
	GrpcURI string

	// Location on the local fs where to store reads.
	// Defaults to os.TempDir()
	CacheDirectory string

	// Keytab is the location of the EOS keytab file.
	Keytab string

	// Authkey is the key that authorizes this client to connect to the GRPC service
	// It's unclear whether this will be the final solution
	Authkey string

	// SecProtocol is the comma separated list of security protocols used by xrootd.
	// For example: "sss, unix"
	SecProtocol string
}

func (opt *Options) init() {
	if opt.ForceSingleUserMode && opt.SingleUsername != "" {
		opt.SingleUsername = "apache"
	}

	if opt.EosBinary == "" {
		opt.EosBinary = "/usr/bin/eos"
	}

	if opt.XrdcopyBinary == "" {
		opt.XrdcopyBinary = "/usr/bin/xrdcopy"
	}

	if opt.URL == "" {
		opt.URL = "root://eos-example.org"
	}

	if opt.CacheDirectory == "" {
		opt.CacheDirectory = os.TempDir()
	}
}

// Client performs actions against a EOS management node (MGM).
// It requires the eos-client and xrootd-client packages installed to work.
type Client struct {
	opt *Options
	cl  erpc.EosClient
}

// New creates a new client with the given options.
func New(opt *Options) *Client {
	opt.init()
	c := new(Client)
	c.opt = opt

	tlog := logger.New().With().Int("pid", os.Getpid()).Logger()
	tctx := appctx.WithLogger(context.Background(), &tlog)
	tlog.Log().Str("ffff", "ddddd").Msg("")

	fmt.Printf("--- Connecting to '%s'\n", opt.GrpcURI)
	conn, err := grpc.Dial(opt.GrpcURI, grpc.WithInsecure())
	if err != nil {
		fmt.Printf("--- Ping to '%s' gave err '%s'\n", opt.GrpcURI, err)
		return nil
	}

	fmt.Printf("--- Going to ping to '%s'\n", opt.GrpcURI)
	c.cl = erpc.NewEosClient(conn)

	// If we can't ping... exit immediately... we will see if this has to be kept, for now it's practical
	prq := new(erpc.PingRequest)
	prq.Authkey = opt.Authkey
	prq.Message = []byte("hi this is a ping from reva")
	prep, err := erpc.EosClient.Ping(c.cl, tctx, prq)
	if err != nil {
		fmt.Printf("--- Ping to '%s' failed with err '%s'\n", opt.GrpcURI, err)
		return nil
	}

	fmt.Printf("--- Ping to '%s' gave response '%s'\n", opt.GrpcURI, prep)

	fmt.Printf("--- Going to stat '%s'\n", "/eos")
	frep, err := c.GetFileInfoByPath(tctx, "furano", "/eos")
	if err != nil {
		fmt.Printf("--- GetFileInfoByPath '%s' failed with err '%s'\n", "/eos", err)
		return nil
	}
	fmt.Printf("--- GetFileInfoByPath to '%s' gave response '%s'\n", "/eos", frep.File)

	fmt.Printf("--- Going to list '%s'\n", "/eos")
	lrep, err := c.List(context.Background(), "furano", "/eos")
	if err != nil {
		fmt.Printf("--- List '%s' failed with err '%s'\n", "/eos", err)
		return nil
	}
	fmt.Printf("--- List to '%s' gave %d entries\n", "/eos", len(lrep))

	fmt.Printf("--- Going to getACLForPath '%s'\n", "/eos/cms")
	arep, err := c.getACLForPath(context.Background(), "furano", "/eos/cms")
	if err != nil {
		fmt.Printf("--- getACLForPath '%s' failed with err '%s'\n", "/eos/cms", err)
		return nil
	}
	for i, s := range arep.Entries {
		fmt.Printf("--- getACLForPath to '%s' gave %d:'%s'\n", "/eos/cms", i, s)
	}

	// Let's be successful if the ping was ok. This is an initialization phase
	// and we enforce the server to be up
	// TBD: some watchdog to automatically reconnect, yet it's not yet clear to me
	//  the behaviour of grpc in the case of failing/restarting servers. To be tested!
	if prep != nil {
		return c
	}

	return nil
}

func (c *Client) getUnixUser(username string) (*gouser.User, error) {
	if c.opt.ForceSingleUserMode {
		username = c.opt.SingleUsername
	}
	return gouser.Lookup(username)
}

// AddACL adds an new acl to EOS with the given aclType.
func (c *Client) AddACL(ctx context.Context, username, path string, a *acl.Entry) error {
	acls, err := c.getACLForPath(ctx, username, path)
	if err != nil {
		return err
	}

	// since EOS Citrine ACLs are is stored with uid, we need to convert username to uid
	// only for users.
	if a.Type == acl.TypeUser {
		a.Qualifier, err = getUID(a.Qualifier)
		if err != nil {
			return err
		}
	}
	err = acls.SetEntry(a.Type, a.Qualifier, a.Permissions)
	if err != nil {
		return err
	}
	sysACL := acls.Serialize()

	// Stuff filename, uid, gid into the MDRequest type
	rq := new(erpc.NSRequest)

	// setting of the sys.acl is only possible from root user
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return err
	}
	rq.Role = new(erpc.RoleId)

	uid, err := strconv.ParseUint(unixUser.Uid, 10, 64)
	if err != nil {
		return err
	}
	rq.Role.Uid = uid
	gid, err := strconv.ParseUint(unixUser.Gid, 10, 64)
	if err != nil {
		return err
	}
	rq.Role.Gid = gid

	rq.Authkey = c.opt.Authkey

	msg := new(erpc.NSRequest_AclRequest)
	msg.Cmd = erpc.NSRequest_AclRequest_ACL_COMMAND(erpc.NSRequest_AclRequest_ACL_COMMAND_value["MODIFY"])
	msg.Type = erpc.NSRequest_AclRequest_ACL_TYPE(erpc.NSRequest_AclRequest_ACL_TYPE_value["SYS_ACL"])
	msg.Recursive = true
	msg.Rule = sysACL

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Acl{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, context.Background(), rq)
	if err != nil {
		fmt.Printf("--- Exec('%s') failed with err '%s'\n", path, err)
		return err
	}

	fmt.Printf("--- MD('%s') gave response '%s'\n", path, resp)
	if resp == nil {
		return errtypes.NotFound(fmt.Sprintf("PAth: %s", path))
	}

	return err

}

// RemoveACL removes the acl from EOS.
func (c *Client) RemoveACL(ctx context.Context, username, path string, aclType string, recipient string) error {
	acls, err := c.getACLForPath(ctx, username, path)
	if err != nil {
		return err
	}

	// since EOS Citrine ACLs are is stored with uid, we need to convert username to uid
	// only for users.

	// since EOS Citrine ACLs are stored with uid, we need to convert username to uid
	if aclType == acl.TypeUser {
		recipient, err = getUID(recipient)
		if err != nil {
			return err
		}
	}
	acls.DeleteEntry(aclType, recipient)
	sysACL := acls.Serialize()

	// Stuff filename, uid, gid into the MDRequest type
	rq := new(erpc.NSRequest)

	// setting of the sys.acl is only possible from root user
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return err
	}
	rq.Role = new(erpc.RoleId)

	uid, err := strconv.ParseUint(unixUser.Uid, 10, 64)
	if err != nil {
		return err
	}
	rq.Role.Uid = uid
	gid, err := strconv.ParseUint(unixUser.Gid, 10, 64)
	if err != nil {
		return err
	}
	rq.Role.Gid = gid

	rq.Authkey = c.opt.Authkey

	msg := new(erpc.NSRequest_AclRequest)
	msg.Cmd = erpc.NSRequest_AclRequest_ACL_COMMAND(erpc.NSRequest_AclRequest_ACL_COMMAND_value["MODIFY"])
	msg.Type = erpc.NSRequest_AclRequest_ACL_TYPE(erpc.NSRequest_AclRequest_ACL_TYPE_value["SYS_ACL"])
	msg.Recursive = true
	msg.Rule = sysACL

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Acl{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, context.Background(), rq)
	if err != nil {
		fmt.Printf("--- Exec('%s') failed with err '%s'\n", path, err)
		return err
	}

	fmt.Printf("--- MD('%s') gave response '%s'\n", path, resp)
	if resp == nil {
		return errtypes.NotFound(fmt.Sprintf("Path: %s", path))
	}

	return err

}

// UpdateACL updates the EOS acl.
func (c *Client) UpdateACL(ctx context.Context, username, path string, a *acl.Entry) error {
	return c.AddACL(ctx, username, path, a)
}

// GetACL for a file
func (c *Client) GetACL(ctx context.Context, username, path, aclType, target string) (*acl.Entry, error) {
	acls, err := c.ListACLs(ctx, username, path)
	if err != nil {
		return nil, err
	}
	for _, a := range acls {
		if a.Type == aclType && a.Qualifier == target {
			return a, nil
		}
	}
	return nil, errtypes.NotFound(fmt.Sprintf("%s:%s", aclType, target))

}

func getUsername(uid string) (string, error) {
	user, err := gouser.LookupId(uid)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

func getUID(username string) (string, error) {
	user, err := gouser.Lookup(username)
	if err != nil {
		return "", err
	}
	return user.Uid, nil
}

// ListACLs returns the list of ACLs present under the given path.
// EOS returns uids/gid for Citrine version and usernames for older versions.
// For Citire we need to convert back the uid back to username.
func (c *Client) ListACLs(ctx context.Context, username, path string) ([]*acl.Entry, error) {
	log := appctx.GetLogger(ctx)

	parsedACLs, err := c.getACLForPath(ctx, username, path)
	if err != nil {
		return nil, err
	}

	acls := []*acl.Entry{}
	for _, acl := range parsedACLs.Entries {
		// since EOS Citrine ACLs are is stored with uid, we need to convert uid to userame
		// TODO map group names as well if acl.Type == "g" ...
		acl.Qualifier, err = getUsername(acl.Qualifier)
		if err != nil {
			log.Warn().Err(err).Str("path", path).Str("username", username).Str("qualifier", acl.Qualifier).Msg("cannot map qualifier to name")
			continue
		}
		acls = append(acls, acl)
	}
	return acls, nil
}

// Common code to create and initialize a NSRequest
func (c *Client) initNSRequest(username string) (*erpc.NSRequest, error) {
	// Stuff filename, uid, gid into the MDRequest type
	rq := new(erpc.NSRequest)

	// setting of the sys.acl is only possible from root user
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return nil, err
	}
	rq.Role = new(erpc.RoleId)

	uid, err := strconv.ParseUint(unixUser.Uid, 10, 64)
	if err != nil {
		return nil, err
	}
	rq.Role.Uid = uid
	gid, err := strconv.ParseUint(unixUser.Gid, 10, 64)
	if err != nil {
		return nil, err
	}
	rq.Role.Gid = gid

	rq.Authkey = c.opt.Authkey

	return rq, nil
}

func (c *Client) getACLForPath(ctx context.Context, username, path string) (*acl.ACLs, error) {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return nil, err
	}

	msg := new(erpc.NSRequest_AclRequest)
	msg.Cmd = erpc.NSRequest_AclRequest_ACL_COMMAND(erpc.NSRequest_AclRequest_ACL_COMMAND_value["LIST"])
	msg.Type = erpc.NSRequest_AclRequest_ACL_TYPE(erpc.NSRequest_AclRequest_ACL_TYPE_value["SYS_ACL"])
	msg.Recursive = true

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Acl{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, context.Background(), rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return nil, err
	}

	if resp == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	log.Info().Str("username", username).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	aclret, err := acl.Parse(resp.Acl.Rule, acl.ShortTextForm)

	// Now loop and build the correct return value

	return aclret, err
}

// GetFileInfoByInode returns the FileInfo by the given inode
func (c *Client) GetFileInfoByInode(ctx context.Context, username string, inode uint64) (*FileInfo, error) {
	log := appctx.GetLogger(ctx)

	// Stuff filename, uid, gid into the MDRequest type
	mdrq := new(erpc.MDRequest)
	mdrq.Type = erpc.TYPE_STAT
	mdrq.Id = new(erpc.MDId)
	mdrq.Id.Ino = inode

	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return nil, err
	}
	mdrq.Role = new(erpc.RoleId)

	uid, err := strconv.ParseUint(unixUser.Uid, 10, 64)
	if err != nil {
		return nil, err
	}
	mdrq.Role.Uid = uid
	gid, err := strconv.ParseUint(unixUser.Gid, 10, 64)
	if err != nil {
		return nil, err
	}
	mdrq.Role.Gid = gid

	mdrq.Authkey = c.opt.Authkey

	// Now send the req and see what happens
	resp, err := erpc.EosClient.MD(c.cl, context.Background(), mdrq)
	if err != nil {
		log.Warn().Err(err).Uint64("inode", inode).Str("err", err.Error())
		return nil, err
	}
	rsp, err := resp.Recv()
	if err != nil {
		log.Warn().Err(err).Uint64("inode", inode).Str("err", err.Error())
		return nil, err
	}

	if rsp == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil response for inode: '%d'", inode))
	}

	log.Info().Uint64("inode", inode).Msg("grpc response")

	return c.grpcMDResponseToFileInfo(rsp)
}

// SetAttr sets an extended attributes on a path.
func (c *Client) SetAttr(ctx context.Context, username string, attr *Attribute, recursive bool, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_SetXAttrRequest)

	var m = map[string][]byte{attr.Key: []byte(attr.Val)}
	msg.Xattrs = m
	msg.Recursive = recursive

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Xattr{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	log.Info().Str("username", username).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	return err

}

// UnsetAttr unsets an extended attribute on a path.
func (c *Client) UnsetAttr(ctx context.Context, username string, attr *Attribute, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_SetXAttrRequest)

	var ktd = []string{attr.Key}
	msg.Keystodelete = ktd

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Xattr{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	log.Info().Str("username", username).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	return err

}

// GetFileInfoByPath returns the FilInfo at the given path
func (c *Client) GetFileInfoByPath(ctx context.Context, username, path string) (*FileInfo, error) {
	log := appctx.GetLogger(ctx)

	// Stuff filename, uid, gid into the MDRequest type
	mdrq := new(erpc.MDRequest)
	mdrq.Type = erpc.TYPE_STAT
	mdrq.Id = new(erpc.MDId)
	mdrq.Id.Path = []byte(path)

	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return nil, err
	}
	mdrq.Role = new(erpc.RoleId)

	uid, err := strconv.ParseUint(unixUser.Uid, 10, 64)
	if err != nil {
		return nil, err
	}
	mdrq.Role.Uid = uid
	gid, err := strconv.ParseUint(unixUser.Gid, 10, 64)
	if err != nil {
		return nil, err
	}
	mdrq.Role.Gid = gid

	mdrq.Authkey = c.opt.Authkey

	// Now send the req and see what happens
	resp, err := erpc.EosClient.MD(c.cl, ctx, mdrq)
	if err != nil {

		fmt.Printf("--- MD('%s') failed with err '%s'\n", path, err)
		return nil, err
	}
	rsp, err := resp.Recv()
	if err != nil {
		fmt.Printf("--- Recv('%s') failed with err '%s'\n", path, err)
		return nil, err
	}

	fmt.Printf("--- MD('%s') gave response '%s'\n", path, rsp)
	if rsp == nil {
		return nil, errtypes.NotFound(fmt.Sprintf("%s:%s", "acltype", path))
	}

	log.Print("MD--------")
	return c.grpcMDResponseToFileInfo(rsp)

}

// GetQuota gets the quota of a user on the quota node defined by path
func (c *Client) GetQuota(ctx context.Context, username, path string) (int, int, error) {
	return 0, 0, errtypes.NotSupported(fmt.Sprintf("%s:%s", "acltype", path))
}

// Touch creates a 0-size,0-replica file in the EOS namespace.
func (c *Client) Touch(ctx context.Context, username, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_TouchRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Touch{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	log.Info().Str("username", username).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	return err

}

// Chown given path
func (c *Client) Chown(ctx context.Context, username, chownUser, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_ChownRequest)
	msg.Owner = new(erpc.RoleId)

	chownunixUser, err := c.getUnixUser(chownUser)
	if err != nil {
		return err
	}

	msg.Owner.Uid, err = strconv.ParseUint(chownunixUser.Uid, 10, 64)
	if err != nil {
		return err
	}

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Chown{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("chownuser", chownUser).Str("path", path).Str("err", err.Error())
		return err
	}

	log.Info().Str("username", username).Str("chownuser", chownUser).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' chownuser: '%s' path: '%s'", username, chownUser, path))
	}

	return err

}

// Chmod given path
func (c *Client) Chmod(ctx context.Context, username, mode, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_ChmodRequest)

	md, err := strconv.ParseUint(mode, 10, 64)
	if err != nil {
		return err
	}
	msg.Mode = int64(md)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Chmod{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("mode", mode).Str("path", path).Str("err", err.Error())
		return err
	}

	log.Info().Str("username", username).Str("mode", mode).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' mode: '%s' path: '%s'", username, mode, path))
	}

	return err

}

// CreateDir creates a directory at the given path
func (c *Client) CreateDir(ctx context.Context, username, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_MkdirRequest)

	// Let's put 750 as permissions, assuming that EOS will apply some mask
	md, err := strconv.ParseUint("0o750", 10, 64)
	if err != nil {
		return err
	}
	msg.Mode = int64(md)
	msg.Recursive = true
	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Mkdir{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	log.Info().Str("username", username).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	return err

}

func (c *Client) rm(ctx context.Context, username, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_UnlinkRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Unlink{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	log.Info().Str("username", username).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	return err

}

func (c *Client) rmdir(ctx context.Context, username, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_RmdirRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Rmdir{msg}

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Exec(c.cl, ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	log.Info().Str("username", username).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	return err
}

// Remove removes the resource at the given path
func (c *Client) Remove(ctx context.Context, username, path string) error {
	log := appctx.GetLogger(ctx)

	nfo, err := c.GetFileInfoByPath(ctx, username, path)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	if nfo.IsDir {
		return c.rmdir(ctx, username, path)
	}

	return c.rm(ctx, username, path)
}

// Rename renames the resource referenced by oldPath to newPath
func (c *Client) Rename(ctx context.Context, username, oldPath, newPath string) error {
	return errtypes.NotFound(fmt.Sprintf("%s:%s", "acltype", newPath))
}

// List the contents of the directory given by path
func (c *Client) List(ctx context.Context, username, path string) ([]*FileInfo, error) {

	// Stuff filename, uid, gid into the MDRequest type
	fdrq := new(erpc.FindRequest)
	fdrq.Maxdepth = 1
	fdrq.Type = erpc.TYPE_LISTING
	fdrq.Id = new(erpc.MDId)
	fdrq.Id.Path = []byte(path)

	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return nil, err
	}
	fdrq.Role = new(erpc.RoleId)

	uid, err := strconv.ParseUint(unixUser.Uid, 10, 64)
	if err != nil {
		return nil, err
	}
	fdrq.Role.Uid = uid
	gid, err := strconv.ParseUint(unixUser.Gid, 10, 64)
	if err != nil {
		return nil, err
	}
	fdrq.Role.Gid = gid

	fdrq.Authkey = c.opt.Authkey

	// Now send the req and see what happens
	resp, err := erpc.EosClient.Find(c.cl, context.Background(), fdrq)
	if err != nil {
		fmt.Printf("--- Find('%s') failed with err '%s'\n", path, err)
		return nil, err
	}

	var mylst []*FileInfo

	for {
		rsp, err := resp.Recv()
		if err != nil {
			if err == io.EOF {
				return mylst, nil
			}

			fmt.Printf("--- Recv('%s') failed with err '%s'\n", path, err)
			return nil, err
		}

		fmt.Printf("--- Find('%s') gave response '%s'\n", path, rsp)
		if rsp == nil {
			return nil, errtypes.NotFound(fmt.Sprintf("%s", path))
		}

		myitem, err := c.grpcMDResponseToFileInfo(rsp)
		if err != nil {
			fmt.Printf("--- Could not convert item. err '%s'\n", err)
			return nil, err
		}
		mylst = append(mylst, myitem)
	}

	return mylst, nil
}

// Read reads a file from the mgm
func (c *Client) Read(ctx context.Context, username, path string) (io.ReadCloser, error) {
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return nil, err
	}
	uuid := uuid.Must(uuid.NewV4())
	rand := "eosread-" + uuid.String()
	localTarget := fmt.Sprintf("%s/%s", c.opt.CacheDirectory, rand)
	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	cmd := exec.CommandContext(ctx, c.opt.XrdcopyBinary, "--nopbar", "--silent", "-f", xrdPath, localTarget, fmt.Sprintf("-OSeos.ruid=%s&eos.rgid=%s", unixUser.Uid, unixUser.Gid))
	_, _, err = c.execute(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return os.Open(localTarget)
}

// Write writes a file to the mgm
func (c *Client) Write(ctx context.Context, username, path string, stream io.ReadCloser) error {
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return err
	}
	fd, err := ioutil.TempFile(c.opt.CacheDirectory, "eoswrite-")
	if err != nil {
		return err
	}
	defer fd.Close()
	defer os.RemoveAll(fd.Name())

	// copy stream to local temp file
	_, err = io.Copy(fd, stream)
	if err != nil {
		return err
	}
	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	cmd := exec.CommandContext(ctx, c.opt.XrdcopyBinary, "--nopbar", "--silent", "-f", fd.Name(), xrdPath, fmt.Sprintf("-ODeos.ruid=%s&eos.rgid=%s", unixUser.Uid, unixUser.Gid))
	_, _, err = c.execute(ctx, cmd)
	return err
}

// ListDeletedEntries returns a list of the deleted entries.
func (c *Client) ListDeletedEntries(ctx context.Context, username string) ([]*DeletedEntry, error) {
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return nil, err
	}
	// TODO(labkode): add protection if slave is configured and alive to count how many files are in the trashbin before
	// triggering the recycle ls call that could break the instance because of unavailable memory.
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", unixUser.Uid, unixUser.Gid, "recycle", "ls", "-m")
	stdout, _, err := c.executeEOS(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return parseRecycleList(stdout)
}

// RestoreDeletedEntry restores a deleted entry.
func (c *Client) RestoreDeletedEntry(ctx context.Context, username, key string) error {
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", unixUser.Uid, unixUser.Gid, "recycle", "restore", key)
	_, _, err = c.executeEOS(ctx, cmd)
	return err
}

// PurgeDeletedEntries purges all entries from the recycle bin.
func (c *Client) PurgeDeletedEntries(ctx context.Context, username string) error {
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", unixUser.Uid, unixUser.Gid, "recycle", "purge")
	_, _, err = c.executeEOS(ctx, cmd)
	return err
}

// ListVersions list all the versions for a given file.
func (c *Client) ListVersions(ctx context.Context, username, p string) ([]*FileInfo, error) {
	basename := path.Base(p)
	versionFolder := path.Join(path.Dir(p), versionPrefix+basename)
	finfos, err := c.List(ctx, username, versionFolder)
	if err != nil {
		// we send back an empty list
		return []*FileInfo{}, nil
	}
	return finfos, nil
}

// RollbackToVersion rollbacks a file to a previous version.
func (c *Client) RollbackToVersion(ctx context.Context, username, path, version string) error {
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", unixUser.Uid, unixUser.Gid, "file", "versions", path, version)
	_, _, err = c.executeEOS(ctx, cmd)
	return err
}

// ReadVersion reads the version for the given file.
func (c *Client) ReadVersion(ctx context.Context, username, p, version string) (io.ReadCloser, error) {
	basename := path.Base(p)
	versionFile := path.Join(path.Dir(p), versionPrefix+basename, version)
	return c.Read(ctx, username, versionFile)
}

func parseRecycleList(raw string) ([]*DeletedEntry, error) {
	entries := []*DeletedEntry{}
	rawLines := strings.Split(raw, "\n")
	for _, rl := range rawLines {
		if rl == "" {
			continue
		}
		entry, err := parseRecycleEntry(rl)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// parse entries like these:
// recycle=ls  recycle-bin=/eos/backup/proc/recycle/ uid=gonzalhu gid=it size=0 deletion-time=1510823151 type=recursive-dir keylength.restore-path=45 restore-path=/eos/scratch/user/g/gonzalhu/.sys.v#.app.ico/ restore-key=0000000000a35100
// recycle=ls  recycle-bin=/eos/backup/proc/recycle/ uid=gonzalhu gid=it size=381038 deletion-time=1510823151 type=file keylength.restore-path=36 restore-path=/eos/scratch/user/g/gonzalhu/app.ico restore-key=000000002544fdb3
func parseRecycleEntry(raw string) (*DeletedEntry, error) {
	partsBySpace := strings.Split(raw, " ")
	restoreKeyPair, partsBySpace := partsBySpace[len(partsBySpace)-1], partsBySpace[:len(partsBySpace)-1]
	restorePathPair := strings.Join(partsBySpace[9:], " ")

	partsBySpace = partsBySpace[:9]
	partsBySpace = append(partsBySpace, restorePathPair)
	partsBySpace = append(partsBySpace, restoreKeyPair)

	kv := getMap(partsBySpace)
	size, err := strconv.ParseUint(kv["size"], 10, 64)
	if err != nil {
		return nil, err
	}
	isDir := false
	if kv["type"] == "recursive-dir" {
		isDir = true
	}
	deletionMTime, err := strconv.ParseUint(strings.Split(kv["deletion-time"], ".")[0], 10, 64)
	if err != nil {
		return nil, err
	}
	entry := &DeletedEntry{
		RestorePath:   kv["restore-path"],
		RestoreKey:    kv["restore-key"],
		Size:          size,
		DeletionMTime: deletionMTime,
		IsDir:         isDir,
	}
	return entry, nil
}

func getMap(partsBySpace []string) map[string]string {
	kv := map[string]string{}
	for _, pair := range partsBySpace {
		parts := strings.Split(pair, "=")
		if len(parts) > 1 {
			kv[parts[0]] = parts[1]
		}

	}
	return kv
}

func (c *Client) parseFind(dirPath, raw string) ([]*FileInfo, error) {
	finfos := []*FileInfo{}
	rawLines := strings.Split(raw, "\n")
	for _, rl := range rawLines {
		if rl == "" {
			continue
		}
		fi, err := c.parseFileInfo(rl)
		if err != nil {
			return nil, err
		}
		// dirs in eos end with a slash, like /eos/user/g/gonzalhu/
		// we skip the current directory as eos find will return the directory we
		// ask to find
		if fi.File == path.Clean(dirPath) {
			continue
		}
		finfos = append(finfos, fi)
	}
	return finfos, nil
}

func (c Client) parseQuotaLine(line string) map[string]string {
	partsBySpace := strings.Split(line, " ")
	m := getMap(partsBySpace)
	return m
}
func (c *Client) parseQuota(path, raw string) (int, int, error) {
	rawLines := strings.Split(raw, "\n")
	for _, rl := range rawLines {
		if rl == "" {
			continue
		}

		m := c.parseQuotaLine(rl)
		// map[maxbytes:2000000000000 maxlogicalbytes:1000000000000 percentageusedbytes:0.49 quota:node uid:gonzalhu space:/eos/scratch/user/ usedbytes:9829986500 usedlogicalbytes:4914993250 statusfiles:ok usedfiles:334 maxfiles:1000000 statusbytes:ok]

		space := m["space"]
		if strings.HasPrefix(path, space) {
			maxBytesString := m["maxlogicalbytes"]
			usedBytesString := m["usedlogicalbytes"]
			maxBytes, _ := strconv.ParseInt(maxBytesString, 10, 64)
			usedBytes, _ := strconv.ParseInt(usedBytesString, 10, 64)
			return int(maxBytes), int(usedBytes), nil
		}
	}
	return 0, 0, nil
}

func (c *Client) grpcMDResponseToFileInfo(st *erpc.MDResponse) (*FileInfo, error) {
	if st.Cmd == nil && st.Fmd == nil {
		return nil, errors.Wrap(errtypes.NotSupported(""), "Invalid response (st.Cmd and st.Fmd are nil)")
	}
	fi := new(FileInfo)

	if st.Type != 0 {
		fi.IsDir = true
	}
	if st.Fmd != nil {
		fi.Inode = st.Fmd.Id
		fi.UID = st.Fmd.Uid
		fi.GID = st.Fmd.Gid
		fi.MTimeSec = st.Fmd.Mtime.Sec
		fi.File = string(st.Fmd.Name)

		for k, v := range st.Fmd.Xattrs {
			if fi.Attrs == nil {
				fi.Attrs = make(map[string]string)
			}
			fi.Attrs[k] = string(v)
		}
		fi.Size = st.Fmd.Size

		fi.Size = st.Fmd.Size

	} else {
		fi.Inode = st.Cmd.Id
		fi.UID = st.Cmd.Uid
		fi.GID = st.Cmd.Gid
		fi.MTimeSec = st.Cmd.Mtime.Sec
		fi.File = string(st.Cmd.Name)

		for k, v := range st.Cmd.Xattrs {
			if fi.Attrs == nil {
				fi.Attrs = make(map[string]string)
			}
			fi.Attrs[k] = string(v)
		}

		fi.Size = 0
	}

	fi.ETag = fi.Attrs["etag"]

	return fi, nil
}

// TODO(labkode): better API to access extended attributes.
func (c *Client) parseFileInfo(raw string) (*FileInfo, error) {

	line := raw[15:]
	index := strings.Index(line, " file=/")
	lengthString := line[0:index]
	length, err := strconv.ParseUint(lengthString, 10, 64)
	if err != nil {
		return nil, err
	}

	line = line[index+6:] // skip ' file='
	name := line[0:length]

	kv := make(map[string]string)
	// strip trailing slash
	kv["file"] = strings.TrimSuffix(name, "/")

	line = line[length+1:]
	partsBySpace := strings.Split(line, " ") // we have [size=45 container=3 ...}
	var previousXAttr = ""
	for _, p := range partsBySpace {
		partsByEqual := strings.Split(p, "=") // we have kv pairs like [size 14]
		if len(partsByEqual) == 2 {
			// handle xattrn and xattrv special cases
			switch {
			case partsByEqual[0] == "xattrn":
				previousXAttr = partsByEqual[1]
			case partsByEqual[0] == "xattrv":
				kv[previousXAttr] = partsByEqual[1]
				previousXAttr = ""
			default:
				kv[partsByEqual[0]] = partsByEqual[1]

			}
		}
	}

	fi, err := c.mapToFileInfo(kv)
	if err != nil {
		return nil, err
	}
	return fi, nil
}

// mapToFileInfo converts the dictionary to an usable structure.
// The kv has format:
// map[sys.forced.space:default files:0 mode:42555 ino:5 sys.forced.blocksize:4k sys.forced.layout:replica uid:0 fid:5 sys.forced.blockchecksum:crc32c sys.recycle:/eos/backup/proc/recycle/ fxid:00000005 pid:1 etag:5:0.000 keylength.file:4 file:/eos treesize:1931593933849913 container:3 gid:0 mtime:1498571294.108614409 ctime:1460121992.294326762 pxid:00000001 sys.forced.checksum:adler sys.forced.nstripes:2]
func (c *Client) mapToFileInfo(kv map[string]string) (*FileInfo, error) {
	inode, err := strconv.ParseUint(kv["ino"], 10, 64)
	if err != nil {
		return nil, err
	}
	fid, err := strconv.ParseUint(kv["fid"], 10, 64)
	if err != nil {
		return nil, err
	}
	uid, err := strconv.ParseUint(kv["uid"], 10, 64)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(kv["gid"], 10, 64)
	if err != nil {
		return nil, err
	}

	var treeSize uint64
	// treeSize is only for containers, so we check
	if val, ok := kv["treesize"]; ok {
		treeSize, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	var fileCounter uint64
	// fileCounter is only for containers
	if val, ok := kv["files"]; ok {
		fileCounter, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	var dirCounter uint64
	// dirCounter is only for containers
	if val, ok := kv["container"]; ok {
		dirCounter, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return nil, err
		}
	}

	// treeCount is the number of entries under the tree
	treeCount := fileCounter + dirCounter

	var size uint64
	if val, ok := kv["size"]; ok {
		size, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return nil, err
		}
	}

	// mtime is split by a dot, we only take the first part, do we need subsec precision?
	mtime := strings.Split(kv["mtime"], ".")
	mtimesec, err := strconv.ParseUint(mtime[0], 10, 64)
	if err != nil {
		return nil, err
	}
	mtimenanos, err := strconv.ParseUint(mtime[1], 10, 32)
	if err != nil {
		return nil, err
	}

	isDir := false
	if _, ok := kv["files"]; ok {
		isDir = true
	}

	fi := &FileInfo{
		File:       kv["file"],
		Inode:      inode,
		FID:        fid,
		UID:        uid,
		GID:        gid,
		ETag:       kv["etag"],
		Size:       size,
		TreeSize:   treeSize,
		MTimeSec:   mtimesec,
		MTimeNanos: uint32(mtimenanos),
		IsDir:      isDir,
		Instance:   c.opt.URL,
		SysACL:     kv["sys.acl"],
		TreeCount:  treeCount,
		Attrs:      kv,
	}

	return fi, nil
}

// FileInfo represents the metadata information returned by querying the EOS namespace.
type FileInfo struct {
	IsDir      bool
	MTimeNanos uint32
	Inode      uint64            `json:"inode"`
	FID        uint64            `json:"fid"`
	UID        uint64            `json:"uid"`
	GID        uint64            `json:"gid"`
	TreeSize   uint64            `json:"tree_size"`
	MTimeSec   uint64            `json:"mtime_sec"`
	Size       uint64            `json:"size"`
	TreeCount  uint64            `json:"tree_count"`
	File       string            `json:"eos_file"`
	ETag       string            `json:"etag"`
	Instance   string            `json:"instance"`
	SysACL     string            `json:"sys_acl"`
	Attrs      map[string]string `json:"attrs"`
}

// DeletedEntry represents an entry from the trashbin.
type DeletedEntry struct {
	RestorePath   string
	RestoreKey    string
	Size          uint64
	DeletionMTime uint64
	IsDir         bool
}

// exec executes the command and returns the stdout, stderr and return code
func (c *Client) execute(ctx context.Context, cmd *exec.Cmd) (string, string, error) {
	log := appctx.GetLogger(ctx)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	cmd.Env = []string{
		"EOS_MGM_URL=" + c.opt.URL,
	}

	if c.opt.UseKeytab {
		cmd.Env = append(cmd.Env, "XrdSecPROTOCOL="+c.opt.SecProtocol)
		cmd.Env = append(cmd.Env, "XrdSecSSSKT="+c.opt.Keytab)
	}

	err := cmd.Run()

	var exitStatus int
	if exiterr, ok := err.(*exec.ExitError); ok {
		// The program has exited with an exit code != 0
		// This works on both Unix and Windows. Although package
		// syscall is generally platform dependent, WaitStatus is
		// defined for both Unix and Windows and in both cases has
		// an ExitStatus() method with the same signature.
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {

			exitStatus = status.ExitStatus()
			switch exitStatus {
			case 0:
				err = nil
			case 2:
				err = errtypes.NotFound(errBuf.String())
			}
		}
	}

	args := fmt.Sprintf("%s", cmd.Args)
	env := fmt.Sprintf("%s", cmd.Env)
	log.Info().Str("args", args).Str("env", env).Int("exit", exitStatus).Msg("eos cmd")

	if err != nil && exitStatus != 2 { // don't wrap the errtypes.NotFoundError
		err = errors.Wrap(err, "eosclient: error while executing command")
	}

	return outBuf.String(), errBuf.String(), err
}

// exec executes only EOS commands the command and returns the stdout, stderr and return code.
// execute() executes arbitrary commands.
func (c *Client) executeEOS(ctx context.Context, cmd *exec.Cmd) (string, string, error) {
	log := appctx.GetLogger(ctx)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	cmd.Env = []string{
		"EOS_MGM_URL=" + c.opt.URL,
	}
	trace := trace.FromContext(ctx).SpanContext().TraceID.String()
	cmd.Args = append(cmd.Args, "--comment", trace)

	err := cmd.Run()

	var exitStatus int
	if exiterr, ok := err.(*exec.ExitError); ok {
		// The program has exited with an exit code != 0
		// This works on both Unix and Windows. Although package
		// syscall is generally platform dependent, WaitStatus is
		// defined for both Unix and Windows and in both cases has
		// an ExitStatus() method with the same signature.
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			exitStatus = status.ExitStatus()
			switch exitStatus {
			case 0:
				err = nil
			case 2:
				err = errtypes.NotFound(errBuf.String())
			case 22:
				// eos reports back error code 22 when the user is not allowed to enter the instance
				err = errtypes.PermissionDenied(errBuf.String())
			}
		}
	}

	args := fmt.Sprintf("%s", cmd.Args)
	env := fmt.Sprintf("%s", cmd.Env)
	log.Info().Str("args", args).Str("env", env).Int("exit", exitStatus).Str("err", errBuf.String()).Msg("eos cmd")

	if err != nil && exitStatus != 2 { // don't wrap the errtypes.NotFoundError
		err = errors.Wrap(err, "eosclient: error while executing command")
	}

	return outBuf.String(), errBuf.String(), err
}
