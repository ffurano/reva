// Copyright 2018-2021 CERN
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

package eoshttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
)

// Options to configure the Client.
type Options struct {

	// HTTP URL of the EOS MGM.
	// Default is https://eos-example.org
	BaseURL string

	// Timeout in seconds for connecting to the service
	ConnectTimeout int

	// Timeout in seconds for sending a request to the service and getting a response
	// Does not include redirections
	RWTimeout int

	// Timeout in seconds for performing an operation. Includes every redirection, retry, etc
	OpTimeout int

	// Max idle conns per Transport
	MaxIdleConns int

	// Max conns per transport per destination host
	MaxConnsPerHost int

	// Max idle conns per transport per destination host
	MaxIdleConnsPerHost int

	// TTL for an idle conn per transport
	IdleConnTimeout int

	// If the URL is https, then we need to configure this client
	// with the usual TLS stuff
	// Defaults are /etc/grid-security/hostcert.pem and /etc/grid-security/hostkey.pem
	ClientCertFile string
	ClientKeyFile  string

	// These will override the defaults, which are common system paths hardcoded
	// in the go x509 implementation (why did they do that?!?!?)
	// of course /etc/grid-security/certificates is NOT in those defaults!
	ClientCADirs  string
	ClientCAFiles string
}

// Init fills the basic fields
func (opt *Options) Init() (*http.Transport, error) {

	if opt.BaseURL == "" {
		opt.BaseURL = "https://eos-example.org"
	}

	if opt.ConnectTimeout == 0 {
		opt.ConnectTimeout = 30
	}
	if opt.RWTimeout == 0 {
		opt.RWTimeout = 180
	}
	if opt.OpTimeout == 0 {
		opt.OpTimeout = 360
	}
	if opt.MaxIdleConns == 0 {
		opt.MaxIdleConns = 100
	}
	if opt.MaxConnsPerHost == 0 {
		opt.MaxConnsPerHost = 64
	}
	if opt.MaxIdleConnsPerHost == 0 {
		opt.MaxIdleConnsPerHost = 8
	}
	if opt.IdleConnTimeout == 0 {
		opt.IdleConnTimeout = 30
	}

	if opt.ClientCertFile == "" {
		opt.ClientCertFile = "/etc/grid-security/hostcert.pem"
	}
	if opt.ClientKeyFile == "" {
		opt.ClientKeyFile = "/etc/grid-security/hostkey.pem"
	}

	if opt.ClientCAFiles != "" {
		os.Setenv("SSL_CERT_FILE", opt.ClientCAFiles)
	}
	if opt.ClientCADirs != "" {
		os.Setenv("SSL_CERT_DIR", opt.ClientCADirs)
	} else {
		os.Setenv("SSL_CERT_DIR", "/etc/grid-security/certificates")
	}

	cert, err := tls.LoadX509KeyPair(opt.ClientCertFile, opt.ClientKeyFile)
	if err != nil {
		return nil, err
	}

	// TODO: the error reporting of http.transport is insufficient
	// we may want to check manually at least the existence of the certfiles
	// The point is that also the error reporting of the context that calls this function
	// is weak
	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
		MaxIdleConns:        opt.MaxIdleConns,
		MaxConnsPerHost:     opt.MaxConnsPerHost,
		MaxIdleConnsPerHost: opt.MaxIdleConnsPerHost,
		IdleConnTimeout:     time.Duration(opt.IdleConnTimeout) * time.Second,
		DisableCompression:  true,
	}

	return t, nil
}

// Client performs HTTP-based tasks (e.g. upload, download)
// against a EOS management node (MGM)
// using the EOS XrdHTTP interface.
// In this module we wrap eos-related behaviour, e.g. headers or r/w retries
type Client struct {
	opt Options

	cl *http.Client
}

// New creates a new client with the given options.
func New(opt *Options, t *http.Transport) *Client {
	log := logger.New().With().Int("pid", os.Getpid()).Logger()
	log.Debug().Str("func", "New").Str("Creating new eoshttp client. opt: ", "'"+fmt.Sprintf("%#v", opt)+"' ").Msg("")

	if opt == nil {
		log.Debug().Str("opt is nil, Error creating http client ", "").Msg("")
		return nil
	}

	c := new(Client)
	c.opt = *opt

	// Let's be successful if the ping was ok. This is an initialization phase
	// and we enforce the server to be up
	log.Debug().Str("func", "newhttp").Str("Connecting to ", "'"+opt.BaseURL+"'").Msg("")

	c.cl = &http.Client{
		Transport: t}

	c.cl.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	if c.cl == nil {
		log.Debug().Str("Error creating http client ", "").Msg("")
		return nil
	}

	return c
}

// Format a human readable line that describes a response
func rspdesc(rsp *http.Response) string {
	desc := "'" + fmt.Sprintf("%d", rsp.StatusCode) + "'" + ": '" + rsp.Status + "'"

	buf := new(bytes.Buffer)
	r := "<none>"
	n, e := buf.ReadFrom(rsp.Body)

	if e != nil {
		r = "Error reading body: '" + e.Error() + "'"
	} else if n > 0 {
		r = buf.String()
	}

	desc += " - '" + r + "'"

	return desc
}

// If the error is not nil, take that
// If there is an error coming from EOS, erturn a descriptive error
func (c *Client) getRespError(rsp *http.Response, err error) error {
	if err != nil {
		return err
	}

	if rsp.StatusCode == 0 {
		return nil
	}

	switch rsp.StatusCode {
	case 0, 200, 201:
		return nil
	case 403:
		return errtypes.PermissionDenied(rspdesc(rsp))
	case 404:
		return errtypes.NotFound(rspdesc(rsp))
	}

	err2 := errtypes.InternalError("Err from EOS: " + rspdesc(rsp))
	return err2
}

// From the basepath and the file path... build an url
func (c *Client) buildFullURL(urlpath, uid, gid string) (string, error) {

	u, err := url.Parse(c.opt.BaseURL)
	if err != nil {
		return "", err
	}

	u, err = u.Parse(urlpath)
	if err != nil {
		return "", err
	}

	// I feel safer putting here a check, to prohibit malicious users to
	// inject a false uid/gid into the url
	// Who knows, maybe it's redundant? Better more than nothing.
	p1 := strings.Index(urlpath, "eos.ruid")
	if p1 > 0 && (urlpath[p1-1] == '&' || urlpath[p1-1] == '?') {
		return "", errtypes.PermissionDenied("Illegal malicious url " + urlpath)
	}
	p1 = strings.Index(urlpath, "eos.guid")
	if p1 > 0 && (urlpath[p1-1] == '&' || urlpath[p1-1] == '?') {
		return "", errtypes.PermissionDenied("Illegal malicious url " + urlpath)
	}

	v := u.Query()

	if len(uid) > 0 {
		v.Set("eos.ruid", uid)
	}
	if len(gid) > 0 {
		v.Set("eos.rgid", gid)
	}

	u.RawQuery = v.Encode()
	return u.String(), nil
}

// GETFile does an entire GET to download a full file. Returns a stream to read the content from
func (c *Client) GETFile(ctx context.Context, httptransport *http.Transport, remoteuser, uid, gid, urlpath string, stream io.WriteCloser) (io.ReadCloser, error) {

	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "GETFile").Str("remoteuser", remoteuser).Str("uid,gid", uid+","+gid).Str("path", urlpath).Msg("")

	// Now send the req and see what happens
	finalurl, err := c.buildFullURL(urlpath, uid, gid)
	if err != nil {
		log.Error().Str("func", "GETFile").Str("url", finalurl).Str("err", err.Error()).Msg("can't create request")
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", finalurl, nil)
	if err != nil {
		log.Error().Str("func", "GETFile").Str("url", finalurl).Str("err", err.Error()).Msg("can't create request")
		return nil, err
	}

	ntries := 0
	nredirs := 0
	timebegin := time.Now().Unix()

	for {
		// Check for a max count of redirections or retries

		// Check for a global timeout in any case
		tdiff := time.Now().Unix() - timebegin
		if tdiff > int64(c.opt.OpTimeout) {
			log.Error().Str("func", "GETFile").Str("url", finalurl).Int64("timeout", tdiff).Int("ntries", ntries).Msg("")
			return nil, errtypes.InternalError("Timeout with url" + finalurl)
		}

		// Execute the request. I don't like that there is no explicit timeout or buffer control on the input stream
		log.Debug().Str("func", "GETFile").Msg("sending req")
		resp, err := c.cl.Do(req)

		// Let's support redirections... and if we retry we have to retry at the same FST, avoid going back to the MGM
		if resp != nil && (resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusTemporaryRedirect) {

			// io.Copy(ioutil.Discard, resp.Body)
			// resp.Body.Close()

			loc, err := resp.Location()
			if err != nil {
				log.Error().Str("func", "GETFile").Str("url", finalurl).Str("err", err.Error()).Msg("can't get a new location for a redirection")
				return nil, err
			}

			c.cl = &http.Client{
				Transport: httptransport}

			req, err = http.NewRequestWithContext(ctx, "GET", loc.String(), nil)
			if err != nil {
				log.Error().Str("func", "GETFile").Str("url", loc.String()).Str("err", err.Error()).Msg("can't create redirected request")
				return nil, err
			}

			req.Close = true

			log.Debug().Str("func", "GETFile").Str("location", loc.String()).Msg("redirection")
			nredirs++
			resp = nil
			err = nil
			continue
		}

		// And get an error code (if error) that is worth propagating
		e := c.getRespError(resp, err)
		if e != nil {
			if os.IsTimeout(e) {
				ntries++
				log.Warn().Str("func", "GETFile").Str("url", finalurl).Str("err", e.Error()).Int("try", ntries).Msg("recoverable network timeout")
				continue
			}
			log.Error().Str("func", "GETFile").Str("url", finalurl).Str("err", e.Error()).Msg("")
			return nil, e
		}

		log.Debug().Str("func", "GETFile").Str("url", finalurl).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("")
		if resp == nil {
			return nil, errtypes.NotFound(fmt.Sprintf("url: %s", finalurl))
		}

		if stream != nil {
			// Streaming versus localfile. If we have bene given a dest stream then copy the body into it
			_, err = io.Copy(stream, resp.Body)
			return nil, err
		}

		// If we have not been given a stream to write into then return our stream to read from
		return resp.Body, nil
	}

}

// PUTFile does an entire PUT to upload a full file, taking the data from a stream
func (c *Client) PUTFile(ctx context.Context, httptransport *http.Transport, remoteuser, uid, gid, urlpath string, stream io.ReadCloser, length int64) error {

	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "PUTFile").Str("remoteuser", remoteuser).Str("uid,gid", uid+","+gid).Str("path", urlpath).Int64("length", length).Msg("")

	// Now send the req and see what happens
	finalurl, err := c.buildFullURL(urlpath, uid, gid)
	if err != nil {
		log.Error().Str("func", "PUTFile").Str("url", finalurl).Str("err", err.Error()).Msg("can't create request")
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", finalurl, nil)
	if err != nil {
		log.Error().Str("func", "PUTFile").Str("url", finalurl).Str("err", err.Error()).Msg("can't create request")
		return err
	}

	req.Close = true

	ntries := 0
	nredirs := 0
	timebegin := time.Now().Unix()

	for {
		// Check for a max count of redirections or retries

		// Check for a global timeout in any case
		tdiff := time.Now().Unix() - timebegin
		if tdiff > int64(c.opt.OpTimeout) {
			log.Error().Str("func", "PUTFile").Str("url", finalurl).Int64("timeout", tdiff).Int("ntries", ntries).Msg("")
			return errtypes.InternalError("Timeout with url" + finalurl)
		}

		// Execute the request. I don't like that there is no explicit timeout or buffer control on the input stream
		log.Debug().Str("func", "PUTFile").Msg("sending req")
		resp, err := c.cl.Do(req)

		// Let's support redirections... and if we retry we retry at the same FST
		if resp != nil && resp.StatusCode == 307 {

			// io.Copy(ioutil.Discard, resp.Body)
			// resp.Body.Close()

			loc, err := resp.Location()
			if err != nil {
				log.Error().Str("func", "PUTFile").Str("url", finalurl).Str("err", err.Error()).Msg("can't get a new location for a redirection")
				return err
			}

			c.cl = &http.Client{
				Transport: httptransport}

			req, err = http.NewRequestWithContext(ctx, "PUT", loc.String(), stream)
			if err != nil {
				log.Error().Str("func", "PUTFile").Str("url", loc.String()).Str("err", err.Error()).Msg("can't create redirected request")
				return err
			}
			if length >= 0 {
				log.Debug().Str("func", "PUTFile").Int64("Content-Length", length).Msg("setting header")
				req.Header.Set("Content-Length", strconv.FormatInt(length, 10))

			}
			if err != nil {
				log.Error().Str("func", "PUTFile").Str("url", loc.String()).Str("err", err.Error()).Msg("can't create redirected request")
				return err
			}
			if length >= 0 {
				log.Debug().Str("func", "PUTFile").Int64("Content-Length", length).Msg("setting header")
				req.Header.Set("Content-Length", strconv.FormatInt(length, 10))

			}

			log.Debug().Str("func", "PUTFile").Str("location", loc.String()).Msg("redirection")
			nredirs++
			resp = nil
			err = nil
			continue
		}

		// And get an error code (if error) that is worth propagating
		e := c.getRespError(resp, err)
		if e != nil {
			if os.IsTimeout(e) {
				ntries++
				log.Warn().Str("func", "PUTFile").Str("url", finalurl).Str("err", e.Error()).Int("try", ntries).Msg("recoverable network timeout")
				continue
			}
			log.Error().Str("func", "PUTFile").Str("url", finalurl).Str("err", e.Error()).Msg("")
			return e
		}

		log.Debug().Str("func", "PUTFile").Str("url", finalurl).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("")
		if resp == nil {
			return errtypes.NotFound(fmt.Sprintf("url: %s", finalurl))
		}

		return nil
	}

}

// Head performs a HEAD req. Useful to check the server
func (c *Client) Head(ctx context.Context, remoteuser, uid, gid, urlpath string) error {

	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "Head").Str("remoteuser", remoteuser).Str("uid,gid", uid+","+gid).Str("path", urlpath).Msg("")

	// Now send the req and see what happens
	finalurl, err := c.buildFullURL(urlpath, uid, gid)
	if err != nil {
		log.Error().Str("func", "Head").Str("url", finalurl).Str("err", err.Error()).Msg("can't create request")
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "HEAD", finalurl, nil)
	if err != nil {
		log.Error().Str("func", "Head").Str("remoteuser", remoteuser).Str("uid,gid", uid+","+gid).Str("url", finalurl).Str("err", err.Error()).Msg("can't create request")
		return err
	}

	ntries := 0

	timebegin := time.Now().Unix()
	for {
		tdiff := time.Now().Unix() - timebegin
		if tdiff > int64(c.opt.OpTimeout) {
			log.Error().Str("func", "Head").Str("url", finalurl).Int64("timeout", tdiff).Int("ntries", ntries).Msg("")
			return errtypes.InternalError("Timeout with url" + finalurl)
		}
		// Execute the request. I don't like that there is no explicit timeout or buffer control on the input stream
		resp, err := c.cl.Do(req)

		// And get an error code (if error) that is worth propagating
		e := c.getRespError(resp, err)
		if e != nil {
			if os.IsTimeout(e) {
				ntries++
				log.Warn().Str("func", "Head").Str("url", finalurl).Str("err", e.Error()).Int("try", ntries).Msg("recoverable network timeout")
				continue
			}
			log.Error().Str("func", "Head").Str("url", finalurl).Str("err", e.Error()).Msg("")
			return e
		}

		log.Debug().Str("func", "Head").Str("url", finalurl).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("")
		if resp == nil {
			return errtypes.NotFound(fmt.Sprintf("url: %s", finalurl))
		}
	}
	// return nil

}
