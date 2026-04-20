// Package files implements a minimal JSON-over-WS file-ops surface for the
// web file browser. There is no SSH/SFTP protocol — ops dispatch directly to
// os/io calls on the agent host.
package files

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

// Op kinds.
const (
	OpList   = "list"
	OpStat   = "stat"
	OpRead   = "read"
	OpWrite  = "write"
	OpMkdir  = "mkdir"
	OpDelete = "delete"
)

// MaxReadBytes is the hard cap on a single read() response, to keep JSON
// payload sizes sane. The client is responsible for chunking larger reads.
const MaxReadBytes int64 = 1 << 20 // 1 MiB

// Request is the decoded form of model.AgentFileOpPayload on the agent side.
type Request struct {
	Op     string
	Path   string
	Data   string // base64 for write
	Mode   uint32
	Offset int64
	Length int64
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
		return doWrite(req.Path, req.Offset, req.Data)
	case OpMkdir:
		return doMkdir(req.Path, req.Mode)
	case OpDelete:
		return doDelete(req.Path)
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
	st := Entry{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    uint32(info.Mode()),
		ModTime: info.ModTime().Unix(),
		IsDir:   info.IsDir(),
	}
	return Response{OK: true, Stat: &st}
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

func doWrite(path string, offset int64, b64 string) Response {
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
	return Response{OK: true}
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
