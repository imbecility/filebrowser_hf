package fbhttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/imbecility/go-huggingface-hub/hfhub"

	"github.com/filebrowser/filebrowser/v2/files"
)

// hfUploadRequest body of POST /api/hf/upload.
type hfUploadRequest struct {
	RepoID           string   `json:"repoId"`
	RepoType         string   `json:"repoType"`
	Paths            []string `json:"paths"` // VFS paths
	PathInRepoPrefix string   `json:"pathInRepoPrefix"`
	CommitMessage    string   `json:"commitMessage"`
	Token            string   `json:"token"`
}

// hfSSEEvent SSE data:.
type hfSSEEvent struct {
	Action     string `json:"action"`
	Message    string `json:"message,omitempty"`
	Path       string `json:"path,omitempty"`
	TotalBytes int64  `json:"totalBytes,omitempty"`
	BytesDone  int64  `json:"bytesDone,omitempty"`
}

// GET /api/hf/token-status
var hfTokenStatusHandler = withUser(func(w http.ResponseWriter, r *http.Request, _ *data) (int, error) {
	return renderJSON(w, r, map[string]bool{"hasToken": os.Getenv("HF_TOKEN") != ""})
})

// POST /api/hf/upload
var hfUploadHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	// SSE requires Flusher support from the server.
	flusher, ok := w.(http.Flusher)
	if !ok {
		return http.StatusInternalServerError, fmt.Errorf("streaming not supported")
	}

	var req hfUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, err
	}
	if req.RepoID == "" {
		return http.StatusBadRequest, fmt.Errorf("repoId is required")
	}
	if req.RepoType == "" {
		req.RepoType = "model"
	}
	if req.CommitMessage == "" {
		req.CommitMessage = "Upload via FileBrowser"
	}
	if len(req.Paths) == 0 {
		return http.StatusBadRequest, fmt.Errorf("paths array is empty")
	}

	// Resolve VFS paths to real paths + build FileCommitOps
	var ops []hfhub.FileCommitOp

	for _, vfsPath := range req.Paths {
		fi, err := files.NewFileInfo(&files.FileOptions{
			Fs:         d.user.Fs,
			Path:       vfsPath,
			Modify:     false,
			Expand:     false,
			ReadHeader: false,
			Checker:    d,
		})
		if err != nil {
			return http.StatusBadRequest, fmt.Errorf("invalid path %q: %w", vfsPath, err)
		}

		realPath := fi.RealPath()

		if fi.IsDir {
			// Recursively collect all files from the directory.
			walkErr := filepath.Walk(realPath, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					if info.Name() == ".git" {
						return filepath.SkipDir
					}
					return nil
				}
				rel, _ := filepath.Rel(realPath, p)
				repoPath := filepath.ToSlash(rel)
				if req.PathInRepoPrefix != "" {
					repoPath = req.PathInRepoPrefix + "/" + repoPath
				}
				ops = append(ops, hfhub.FileCommitOp{
					LocalPath:  p,
					PathInRepo: repoPath,
				})
				return nil
			})
			if walkErr != nil {
				return http.StatusInternalServerError, fmt.Errorf("scanning %q: %w", vfsPath, walkErr)
			}
		} else {
			repoPath := filepath.Base(realPath)
			if req.PathInRepoPrefix != "" {
				repoPath = req.PathInRepoPrefix + "/" + repoPath
			}
			ops = append(ops, hfhub.FileCommitOp{
				LocalPath:  realPath,
				PathInRepo: repoPath,
			})
		}
	}

	if len(ops) == 0 {
		return http.StatusBadRequest, fmt.Errorf("no files found in selected paths")
	}

	token := os.Getenv("HF_TOKEN")
	if token == "" {
		token = req.Token
	}
	if token == "" {
		return http.StatusUnauthorized, fmt.Errorf("HF_TOKEN not set and no token provided in request")
	}

	// If token is provided in request, set it in env
	if os.Getenv("HF_TOKEN") == "" {
		os.Setenv("HF_TOKEN", token) //nolint:errcheck
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	var mu sync.Mutex
	send := func(ev hfSSEEvent) {
		b, _ := json.Marshal(ev)
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	client := hfhub.NewHubClient()

	err := client.UploadFiles(req.RepoID, req.RepoType, ops, &hfhub.UploadOptions{
		CommitMessage:  req.CommitMessage,
		AutoCreateRepo: true,
		OnProgress: func(ev hfhub.ProgressEvent) {
			send(hfSSEEvent{
				Action:     string(ev.Action),
				Message:    ev.Message,
				Path:       ev.Path,
				TotalBytes: ev.TotalBytes,
				BytesDone:  ev.BytesDone,
			})
		},
	})

	if err != nil {
		send(hfSSEEvent{Action: "error", Message: err.Error()})
	} else {
		send(hfSSEEvent{Action: "complete"})
	}

	return 0, nil
})

// PUT /api/hf/token {"token":"hf_xxx"}
var hfSetTokenHandler = withUser(func(w http.ResponseWriter, r *http.Request, _ *data) (int, error) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return http.StatusBadRequest, err
	}
	if body.Token == "" {
		return http.StatusBadRequest, fmt.Errorf("token is empty")
	}
	os.Setenv("HF_TOKEN", body.Token) //nolint:errcheck
	return renderJSON(w, r, map[string]bool{"ok": true})
})
