// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package goblet

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/google/gitprotocolio"
	"go.opencensus.io/tag"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type httpProxyServer struct {
	config *ServerConfig
}

func (s *httpProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 使用 zap 记录器替代 httpErrorReporter
	logger := zap.S().With(
		"url", r.URL.String(),
		"method", r.Method,
		"remote_addr", r.RemoteAddr,
	)

	ctx, err := tag.New(r.Context(), tag.Insert(CommandTypeKey, "not-a-command"))
	if err != nil {
		logger.Errorw("获取上下文失败", "error", err)
		return
	}
	r = r.WithContext(ctx)

	// Technically, this server is an HTTP proxy, and it should use
	// Proxy-Authorization / Proxy-Authenticate. However, existing
	// authentication mechanism around Git is not compatible with proxy
	// authorization. We use normal authentication mechanism here.
	if err := s.config.RequestAuthorizer(r); err != nil {
		logger.Errorw("请求授权失败", "error", err)
		return
	}
	if proto := r.Header.Get("Git-Protocol"); proto != "version=2" {
		logger.Errorw("仅支持 Git protocol v2", "protocol", proto)
		return
	}

	switch {
	case strings.HasSuffix(r.URL.Path, "/info/refs"):
		s.infoRefsHandler(logger, w, r)
	case strings.HasSuffix(r.URL.Path, "/git-receive-pack"):
		logger.Error("不支持 git-receive-pack")
	case strings.HasSuffix(r.URL.Path, "/git-upload-pack"):
		s.uploadPackHandler(logger, w, r)
	}
}

func (s *httpProxyServer) infoRefsHandler(logger *zap.SugaredLogger, w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("service") != "git-upload-pack" {
		logger.Errorw("仅支持 git-fetch", "service", r.URL.Query().Get("service"))
		return
	}

	w.Header().Add("Content-Type", "application/x-git-upload-pack-advertisement")
	rs := []*gitprotocolio.InfoRefsResponseChunk{
		{ProtocolVersion: 2},
		{Capabilities: []string{"ls-refs"}},
		// See managed_repositories.go for not having ref-in-want.
		{Capabilities: []string{"fetch=filter shallow"}},
		{Capabilities: []string{"server-option"}},
		{EndOfRequest: true},
	}
	for _, pkt := range rs {
		if err := writePacket(w, pkt); err != nil {
			// Client-side IO error. Treat this as Canceled.
			logger.Errorw("客户端 IO 错误", "error", err)
			return
		}
	}
}

func (s *httpProxyServer) uploadPackHandler(logger *zap.SugaredLogger, w http.ResponseWriter, r *http.Request) {
	// /git-upload-pack doesn't recognize text/plain error. Send an error
	// with ErrorPacket.
	w.Header().Add("Content-Type", "application/x-git-upload-pack-result")
	if r.Header.Get("Content-Encoding") == "gzip" {
		var err error
		if r.Body, err = gzip.NewReader(r.Body); err != nil {
			logger.Errorw("无法解压 gzip", "error", err)
			return
		}
	}

	// HTTP is strictly speaking a request-response protocol, and a server
	// cannot send a non-error response until the entire request is read.
	// We need to compromise and either drain the entire request first or
	// buffer the entire response.
	//
	// Because this server supports only ls-refs and fetch commands, valid
	// protocol V2 requests are relatively small in practice compared to the
	// response. A request with many wants and haves can be large, but
	// practically there's a limit on the number of haves a client would
	// send. Compared to that the fetch response can contain a packfile, and
	// this can easily get large. Read the entire request upfront.
	commands, err := parseAllCommands(r.Body)
	if err != nil {
		logger.Errorw("无法解析命令", "error", err)
		return
	}

	repo, err := openManagedRepository(s.config, r.URL)
	if err != nil {
		logger.Errorw("无法打开管理的仓库", "error", err)
		return
	}

	for _, command := range commands {
		if !handleV2Command(r.Context(), repo, command, w) {
			return
		}
	}
}

func parseAllCommands(r io.Reader) ([][]*gitprotocolio.ProtocolV2RequestChunk, error) {
	commands := [][]*gitprotocolio.ProtocolV2RequestChunk{}
	v2Req := gitprotocolio.NewProtocolV2Request(r)
	for {
		chunks := []*gitprotocolio.ProtocolV2RequestChunk{}
		for v2Req.Scan() {
			c := copyRequestChunk(v2Req.Chunk())
			if c.EndRequest {
				break
			}
			chunks = append(chunks, c)
		}
		if len(chunks) == 0 || v2Req.Err() != nil {
			break
		}

		switch chunks[0].Command {
		case "ls-refs":
		case "fetch":
			// Do nothing.
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unrecognized command: %v", chunks[0])
		}
		commands = append(commands, chunks)
	}

	if err := v2Req.Err(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "cannot parse the request: %v", err)
	}
	return commands, nil
}
