// Package files implements a minimal JSON-over-WS file-ops surface for the
// web file browser. There is no SSH/SFTP protocol — ops dispatch directly to
// os/io calls on the agent host.
package files

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Op kinds.
const (
	OpList   = "list"
	OpStat   = "stat"
	OpRead   = "read"
	OpWrite  = "write"
	OpMkdir  = "mkdir"
	OpDelete = "delete"
	OpCommit = "commit"
)

// MaxReadBytes is the hard cap on a single read() response, to keep JSON
// payload sizes sane. The client is responsible for chunking larger reads.
const MaxReadBytes int64 = 1 << 20 // 1 MiB

// Request is the decoded form of model.AgentFileOpPayload on the agent side.
type Request struct {
	Op     string
	Path   string
	Data   string // base64 for write
	Mode   uint32 // permission bits, for mkdir, write and commit
	Offset int64
	Length int64
	From   string // for commit: the staged file to move to Path
	SHA256 string // for commit: hex digest From must have
}

// Entry is a directory entry sent back in Response.Entries.
type Entry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    uint32 `json:"mode"`
	ModTime int64  `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
}

// Response mirrors model.AgentFileResultPayload (the server will re-wrap it).
type Response struct {
	OK      bool    `json:"ok"`
	Error   string  `json:"error,omitempty"`
	Entries []Entry `json:"entries,omitempty"`
	Data    string  `json:"data,omitempty"`
	Stat    *Entry  `json:"stat,omitempty"`
	EOF     bool    `json:"eof,omitempty"`
	SHA256  string  `json:"sha256,omitempty"` // for commit: the digest it computed
}

// Handle dispatches a single file op and returns the response.
// Never returns an error: failures are represented via Response.OK=false + Error.
func Handle(req Request) Response {
	if req.Path == "" {
		return errResp(errors.New("path is required"))
	}

	switch req.Op {
	case OpList:
		return doList(req.Path)
	case OpStat:
		return doStat(req.Path)
	case OpRead:
		return doRead(req.Path, req.Offset, req.Length)
	case OpWrite:
		return doWrite(req.Path, req.Offset, req.Data, req.Mode)
	case OpMkdir:
		return doMkdir(req.Path, req.Mode)
	case OpDelete:
		return doDelete(req.Path)
	case OpCommit:
		return doCommit(req.From, req.Path, req.SHA256, req.Mode)
	default:
		return errResp(fmt.Errorf("unknown op: %q", req.Op))
	}
}

func doList(path string) Response {
	entries, err := os.ReadDir(path)
	if err != nil {
		return errResp(err)
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Entry{
			Name:    e.Name(),
			Size:    info.Size(),
			Mode:    uint32(info.Mode()),
			ModTime: info.ModTime().Unix(),
			IsDir:   e.IsDir(),
		})
	}
	return Response{OK: true, Entries: out}
}

func doStat(path string) Response {
	info, err := os.Stat(path)
	if err != nil {
		return errResp(err)
	}
	return Response{OK: true, Stat: entryOf(info)}
}

func entryOf(info os.FileInfo) *Entry {
	return &Entry{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    uint32(info.Mode()),
		ModTime: info.ModTime().Unix(),
		IsDir:   info.IsDir(),
	}
}

func doRead(path string, offset, length int64) Response {
	if length <= 0 || length > MaxReadBytes {
		length = MaxReadBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return errResp(err)
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return errResp(err)
		}
	}

	buf := make([]byte, length)
	n, err := f.Read(buf)
	eof := false
	if err != nil {
		// io.EOF at offset==size is legitimate → treat as empty read with EOF=true.
		if n == 0 && err.Error() == "EOF" {
			eof = true
		} else if err.Error() != "EOF" {
			return errResp(err)
		} else {
			eof = true
		}
	}
	// Check if we hit EOF exactly at buffer boundary.
	if !eof {
		// Peek one more byte to discover EOF at boundary without allocating.
		var one [1]byte
		m, _ := f.Read(one[:])
		if m == 0 {
			eof = true
		}
	}

	return Response{
		OK:   true,
		Data: base64.StdEncoding.EncodeToString(buf[:n]),
		EOF:  eof,
	}
}

func doWrite(path string, offset int64, b64 string, mode uint32) Response {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return errResp(err)
	}

	flag := os.O_WRONLY | os.O_CREATE
	if offset == 0 {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return errResp(err)
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return errResp(err)
		}
	}
	if _, err := f.Write(raw); err != nil {
		return errResp(err)
	}
	// OpenFile's perm only applies on create, and through the umask.
	if mode != 0 {
		if err := f.Chmod(os.FileMode(mode).Perm()); err != nil {
			return errResp(err)
		}
	}
	return Response{OK: true}
}

// doCommit puts a staged upload in place. It hashes from, refuses it if want
// is given and differs, applies mode, and renames it over path. A rename
// within one filesystem is atomic, so anything opening path sees either the
// old file or the complete new one, never a half-written one; and replacing a
// running binary works, where writing into it fails with "text file busy".
//
// A mismatched file is removed: no later op can repair it, and leaving it
// would only leave junk next to the destination.
func doCommit(from, path, want string, mode uint32) Response {
	if from == "" {
		return errResp(errors.New("from is required"))
	}
	if want != "" {
		if b, err := hex.DecodeString(want); err != nil || len(b) != sha256.Size {
			return errResp(fmt.Errorf("sha256 %q is not a hex sha256 digest", want))
		}
	}

	f, err := os.Open(from)
	if err != nil {
		return errResp(err)
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	// Flush before the rename makes the file visible, so a crash right after
	// can't leave an empty file under the real name. Best effort: Windows
	// refuses to flush a handle opened read-only.
	_ = f.Sync()
	f.Close() // before the rename, which Windows refuses on an open file
	if err != nil {
		return errResp(err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if want != "" && !strings.EqualFold(got, want) {
		_ = os.Remove(from)
		return Response{
			Error:  fmt.Sprintf("sha256 mismatch: %s is %s, expected %s; removed it", from, got, strings.ToLower(want)),
			SHA256: got,
		}
	}

	perm := os.FileMode(mode).Perm()
	if mode == 0 {
		// Replacing a file keeps its permissions, so updating an executable
		// leaves it executable.
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
			perm = st.Mode().Perm()
		}
	}
	if perm != 0 {
		if err := os.Chmod(from, perm); err != nil {
			return errResp(err)
		}
	}
	if err := os.Rename(from, path); err != nil {
		return errResp(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return errResp(err)
	}
	return Response{OK: true, Stat: entryOf(info), SHA256: got}
}

func doMkdir(path string, mode uint32) Response {
	if mode == 0 {
		mode = 0o755
	}
	if err := os.MkdirAll(path, os.FileMode(mode)); err != nil {
		return errResp(err)
	}
	return Response{OK: true}
}

func doDelete(path string) Response {
	if err := os.Remove(path); err != nil {
		return errResp(err)
	}
	return Response{OK: true}
}

func errResp(err error) Response {
	return Response{OK: false, Error: err.Error()}
}
