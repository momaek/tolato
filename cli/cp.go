package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// cpChunk is how much of a file goes in one request. Base64 grows it to about
// 700 KB of JSON, which stays under nginx's default 1 MB body limit for
// servers sitting behind one.
const cpChunk = 512 << 10

// partSuffix marks an upload in progress. The data lands in <dst>.tolato-part
// and is moved over <dst> only once all of it arrived, so an interrupted copy
// never leaves a half-written file under the real name, and replacing a
// running binary works (writing into it in place fails with "text file busy").
const partSuffix = ".tolato-part"

// runCp copies one file between this machine and a node. Exactly one side is
// remote, written node:/absolute/path; `-` stands for stdin or stdout.
func runCp(c *client, args []string) error {
	fs := flag.NewFlagSet("cp", flag.ContinueOnError)
	confirm := fs.Bool("confirm", false, "proceed if the server flags the final move as sensitive")

	// Accept flags before, between or after the two paths.
	var paths []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		paths, rest = append(paths, rest[0]), rest[1:]
	}
	if len(paths) != 2 {
		return errUsage
	}

	srcNode, srcPath, srcRemote := splitRemote(paths[0])
	dstNode, dstPath, dstRemote := splitRemote(paths[1])
	switch {
	case srcRemote && dstRemote:
		return errors.New("copying between two nodes is not supported; copy through this machine")
	case !srcRemote && !dstRemote:
		return errors.New("one side must be remote, written node:/path")
	}

	ctx := context.Background()
	if dstRemote {
		node, err := c.resolveNode(ctx, dstNode)
		if err != nil {
			return err
		}
		return upload(ctx, c, node, srcPath, dstPath, *confirm)
	}
	node, err := c.resolveNode(ctx, srcNode)
	if err != nil {
		return err
	}
	return download(ctx, c, node, srcPath, dstPath)
}

// splitRemote splits node:path. Like scp, a colon only marks a remote path when
// no slash comes before it, so ./a:b is a local file. A Windows drive letter
// (C:\x) is local too.
func splitRemote(arg string) (node, path string, remote bool) {
	i := strings.IndexByte(arg, ':')
	if i <= 0 || strings.ContainsAny(arg[:i], `/\`) || filepath.VolumeName(arg) != "" {
		return "", arg, false
	}
	return arg[:i], arg[i+1:], true
}

// checkRemotePath rejects relative paths. The agent does plain file calls with
// no shell, so there is no ~ to expand and no working directory worth relying
// on.
func checkRemotePath(p string, windows bool) error {
	abs := strings.HasPrefix(p, "/")
	if windows {
		abs = len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') || strings.HasPrefix(p, `\\`)
	}
	if !abs {
		return fmt.Errorf("remote path %q must be absolute (~ and relative paths are not expanded)", p)
	}
	return nil
}

// remoteBase is the last element of a remote path, whichever slash it uses.
func remoteBase(p string) string {
	p = strings.TrimRight(p, `/\`)
	return p[strings.LastIndexAny(p, `/\`)+1:]
}

func upload(ctx context.Context, c *client, node Node, local, remote string, confirm bool) error {
	windows := isWindows(node)
	if err := checkRemotePath(remote, windows); err != nil {
		return err
	}

	var (
		src  io.Reader   = os.Stdin
		name             = "stdin"
		size int64       = -1
		perm os.FileMode = 0o644
	)
	if local != "-" {
		f, err := os.Open(local)
		if err != nil {
			return err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return err
		}
		if st.IsDir() {
			return fmt.Errorf("%s is a directory; only single files can be copied", local)
		}
		src, name, size, perm = f, filepath.Base(local), st.Size(), st.Mode().Perm()
	}

	// Copying into a directory keeps the source's name, as cp does.
	dst := remote
	if st, err := c.File(ctx, node.ID, FileOp{Op: "stat", Path: remote}); err == nil && st.Stat != nil && st.Stat.IsDir {
		dst = strings.TrimRight(remote, `/\`) + "/" + name
	} else if strings.HasSuffix(remote, "/") {
		return fmt.Errorf("%s:%s is not a directory", node.DisplayName(), remote)
	}
	if local == "-" && dst != remote {
		return errors.New("copying stdin needs a file name, not a directory")
	}

	part := dst + partSuffix
	cleanup := func() { _, _ = c.File(ctx, node.ID, FileOp{Op: "delete", Path: part}) }

	prog := newProgress(name, size)
	buf := make([]byte, cpChunk)
	var off int64
	for {
		n, rerr := io.ReadFull(src, buf)
		// The first write goes out even for an empty file: offset 0 is what
		// creates (or truncates) the part file.
		if n > 0 || off == 0 {
			op := FileOp{Op: "write", Path: part, Offset: off, Data: base64.StdEncoding.EncodeToString(buf[:n])}
			if _, err := c.File(ctx, node.ID, op); err != nil {
				prog.done()
				if off > 0 {
					cleanup()
				}
				return fmt.Errorf("write %s:%s: %w", node.DisplayName(), part, err)
			}
			off += int64(n)
			prog.update(off)
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			prog.done()
			cleanup()
			return rerr
		}
	}
	prog.done()

	st, err := c.File(ctx, node.ID, FileOp{Op: "stat", Path: part})
	if err != nil || st.Stat == nil || st.Stat.Size != off {
		cleanup()
		return fmt.Errorf("upload to %s:%s did not arrive intact", node.DisplayName(), part)
	}

	// The agent has no rename, so the final move goes through exec. That also
	// puts it in the audit log next to the write.
	var cmd string
	if windows {
		cmd = fmt.Sprintf(`move /Y "%s" "%s"`, part, dst)
	} else {
		cmd = fmt.Sprintf("chmod %o %s && mv -f %s %s", perm, shellQuote(part), shellQuote(part), shellQuote(dst))
	}
	res, err := c.Exec(ctx, node.ID, cmd, 60, confirm)
	if err != nil {
		cleanup()
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.IsSensitive() {
			return fmt.Errorf("%s\nRe-run with --confirm if you are sure", apiErr.Message)
		}
		return err
	}
	if res.ExitCode != 0 {
		cleanup()
		return fmt.Errorf("move into place on %s failed: %s", node.DisplayName(), strings.TrimSpace(res.Stderr+res.Stdout))
	}
	return nil
}

func download(ctx context.Context, c *client, node Node, remote, local string) error {
	if err := checkRemotePath(remote, isWindows(node)); err != nil {
		return err
	}
	res, err := c.File(ctx, node.ID, FileOp{Op: "stat", Path: remote})
	if err != nil {
		return fmt.Errorf("%s:%s: %w", node.DisplayName(), remote, err)
	}
	if res.Stat == nil {
		return fmt.Errorf("%s:%s: no stat in reply", node.DisplayName(), remote)
	}
	if res.Stat.IsDir {
		return fmt.Errorf("%s:%s is a directory; only single files can be copied", node.DisplayName(), remote)
	}
	remoteSize := res.Stat.Size

	if local == "-" {
		_, err := readRemote(ctx, c, node, remote, os.Stdout, nil)
		return err
	}

	dst := local
	if st, err := os.Stat(local); err == nil && st.IsDir() {
		dst = filepath.Join(local, remoteBase(remote))
	} else if strings.HasSuffix(local, string(filepath.Separator)) || strings.HasSuffix(local, "/") {
		return fmt.Errorf("%s is not a directory", local)
	}

	// Same idea as the upload's part file: nothing appears under the real
	// name until every byte is here.
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+partSuffix+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed

	prog := newProgress(remoteBase(remote), remoteSize)
	_, err = readRemote(ctx, c, node, remote, tmp, prog)
	prog.done()
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}

	perm := os.FileMode(res.Stat.Mode).Perm()
	if perm == 0 {
		perm = 0o644
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// readRemote streams a remote file into w chunk by chunk until the agent
// reports EOF, and returns how many bytes it wrote.
func readRemote(ctx context.Context, c *client, node Node, remote string, w io.Writer, prog *progress) (int64, error) {
	var off int64
	for {
		res, err := c.File(ctx, node.ID, FileOp{Op: "read", Path: remote, Offset: off, Length: cpChunk})
		if err != nil {
			return off, fmt.Errorf("read %s:%s: %w", node.DisplayName(), remote, err)
		}
		data, err := base64.StdEncoding.DecodeString(res.Data)
		if err != nil {
			return off, fmt.Errorf("read %s:%s: %w", node.DisplayName(), remote, err)
		}
		if _, err := w.Write(data); err != nil {
			return off, err
		}
		off += int64(len(data))
		prog.update(off)
		if res.EOF {
			return off, nil
		}
		// A reply with no bytes and no EOF would loop forever.
		if len(data) == 0 {
			return off, fmt.Errorf("read %s:%s: agent returned no data at offset %d", node.DisplayName(), remote, off)
		}
	}
}

// progress draws a one-line transfer meter on stderr, only when stderr is a
// terminal so scripts and logs stay clean. A nil *progress draws nothing.
type progress struct {
	name  string
	total int64
}

func newProgress(name string, total int64) *progress {
	st, err := os.Stderr.Stat()
	if err != nil || st.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	return &progress{name: name, total: total}
}

func (p *progress) update(n int64) {
	if p == nil {
		return
	}
	if p.total > 0 {
		fmt.Fprintf(os.Stderr, "\r%s  %s / %s  %3d%%", p.name, humanBytes(n), humanBytes(p.total), n*100/p.total)
	} else {
		fmt.Fprintf(os.Stderr, "\r%s  %s", p.name, humanBytes(n))
	}
}

func (p *progress) done() {
	if p != nil {
		fmt.Fprintln(os.Stderr)
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
