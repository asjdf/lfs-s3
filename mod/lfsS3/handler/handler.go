package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/asjdf/lfs-s3/mod/lfsS3/pkg/auth"
	"github.com/asjdf/lfs-s3/mod/lfsS3/storage"
	"github.com/juanjiTech/jin"
	"github.com/juanjiTech/jin/render"
)

const (
	ContentType = "application/vnd.git-lfs+json"
)

type LFSObject struct {
	OID           string `json:"oid"`
	Size          int64  `json:"size"`
	Authenticated bool   `json:"authenticated,omitempty"`
}

type LFSBatchRequest struct {
	Operation string      `json:"operation"`
	Transfers []string    `json:"transfers,omitempty"`
	Ref       *LFSRef     `json:"ref,omitempty"`
	Objects   []LFSObject `json:"objects"`
	HashAlgo  string      `json:"hash_algo,omitempty"` // 可选，默认 sha256
}

type LFSRef struct {
	Name string `json:"name"`
}

type LFSBatchResponse struct {
	Transfer string              `json:"transfer,omitempty"`
	Objects  []LFSObjectResponse `json:"objects"`
	HashAlgo string              `json:"hash_algo,omitempty"`
}

type LFSObjectAction struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresIn int               `json:"expires_in,omitempty"`
}

type LFSObjectResponse struct {
	OID           string `json:"oid"`
	Size          int64  `json:"size"`
	Authenticated bool   `json:"authenticated,omitempty"` // Optional boolean specifying whether the request for this specific object is authenticated. If omitted or false, Git LFS will attempt to find credentials for this URL.
	Actions       struct {
		Download *LFSObjectAction `json:"download,omitempty"`
		Upload   *LFSObjectAction `json:"upload,omitempty"`
	} `json:"actions"`
	Error *LFSObjectError `json:"error,omitempty"`
}

type LFSObjectError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type LFSResponseError struct {
	Message          string `json:"message"`
	RequestID        string `json:"request_id,omitempty"`
	DocumentationURL string `json:"documentation_url,omitempty"`
}

type Handler struct {
	storage    *storage.S3Storage
	authorizer *auth.Authorizer
}

func NewHandler(s *storage.S3Storage, a *auth.Authorizer) *Handler {
	return &Handler{
		storage:    s,
		authorizer: a,
	}
}

func (h *Handler) RegisterRoutes(e *jin.Engine) {
	e.POST("/:repoOwner/:repoName/info/lfs/objects/batch", h.handleBatch)
	e.NoRoute(func(c *jin.Context) {
		fmt.Println(c.Request.URL.Path)
	})
}

func (h *Handler) handleBatch(c *jin.Context) {
	// 设置响应头
	c.Writer.Header().Set("Content-Type", ContentType)

	//// 鉴权
	//if err := h.authorizer.RequestAuthorizer(c.Request); err != nil {
	//	c.Render(http.StatusUnauthorized, render.JSON{Data: LFSResponseError{
	//		Message:          "Authentication required",
	//		DocumentationURL: "https://github.com/git-lfs/git-lfs/blob/main/docs/api/batch.md",
	//	}})
	//	return
	//}

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Render(http.StatusBadRequest, render.JSON{Data: LFSResponseError{
			Message:          "Failed to read request body",
			DocumentationURL: "https://github.com/git-lfs/git-lfs/blob/main/docs/api/batch.md",
		}})
		return
	}
	defer c.Request.Body.Close()

	var req LFSBatchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.Render(http.StatusBadRequest, render.JSON{Data: LFSResponseError{
			Message:          "Invalid JSON format",
			DocumentationURL: "https://github.com/git-lfs/git-lfs/blob/main/docs/api/batch.md",
		}})
		return
	}

	// 验证必需字段
	if req.Operation == "" {
		c.Render(http.StatusBadRequest, render.JSON{Data: LFSResponseError{
			Message:          "operation is required",
			DocumentationURL: "https://github.com/git-lfs/git-lfs/blob/main/docs/api/batch.md",
		}})
		return
	}

	if len(req.Objects) == 0 {
		c.Render(http.StatusBadRequest, render.JSON{Data: LFSResponseError{
			Message:          "objects array is required and must not be empty",
			DocumentationURL: "https://github.com/git-lfs/git-lfs/blob/main/docs/api/batch.md",
		}})
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(c.Request.URL.Path, "/"), "/")
	if len(pathParts) < 2 {
		c.Render(http.StatusBadRequest, render.JSON{Data: LFSResponseError{
			Message:          "Invalid path",
			DocumentationURL: "https://github.com/git-lfs/git-lfs/blob/main/docs/api/batch.md",
		}})
		return
	}
	repoOwner, repoName := pathParts[0], pathParts[1]

	resp := LFSBatchResponse{
		Transfer: "basic", // 默认使用 basic 传输适配器
		Objects:  make([]LFSObjectResponse, len(req.Objects)),
		HashAlgo: req.HashAlgo,
	}

	for i, obj := range req.Objects {
		respObj := LFSObjectResponse{
			OID:           obj.OID,
			Size:          obj.Size,
			Authenticated: true,
		}

		// 根据操作类型生成相应的预签名URL
		expiresIn := 1 * time.Hour
		var url string
		var err error

		switch req.Operation {
		case "download":
			url, err = h.storage.GetObjectDownloadURL(c.Request.Context(), GenKey(repoOwner, repoName, obj.OID), expiresIn)
			if err == nil {
				respObj.Actions.Download = &LFSObjectAction{
					Href:      url,
					ExpiresIn: int(expiresIn.Seconds()),
				}
			}
		case "upload":
			url, err = h.storage.GetObjectUploadURL(c.Request.Context(), GenKey(repoOwner, repoName, obj.OID), expiresIn)
			if err == nil {
				respObj.Actions.Upload = &LFSObjectAction{
					Href:      url,
					ExpiresIn: int(expiresIn.Seconds()),
				}
			}
		default:
			err = fmt.Errorf("unsupported operation: %s", req.Operation)
		}

		if err != nil {
			respObj.Error = &LFSObjectError{
				Code:    http.StatusInternalServerError,
				Message: err.Error(),
			}
		}

		resp.Objects[i] = respObj
	}

	c.Render(http.StatusOK, render.JSON{Data: resp})
}

func GenKey(org, repo, oid string) string {
	return fmt.Sprintf("%s/%s/%s", org, repo, oid)
}
