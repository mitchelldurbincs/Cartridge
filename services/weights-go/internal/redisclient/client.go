package redisclient

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Options configures the Redis client behaviour.
type Options struct {
	Address  string
	Password string
	Database int
	Timeout  time.Duration
}

// Client provides minimal Redis command execution for SET/GET/LPUSH/LTRIM/PUBLISH.
type Client struct {
	addr     string
	password string
	database int
	timeout  time.Duration
}

// New constructs a Client from options, parsing redis:// URLs if supplied.
func New(opts Options) (*Client, error) {
	if opts.Address == "" {
		return nil, errors.New("redisclient: address is required")
	}

	address := opts.Address
	password := opts.Password
	database := opts.Database

	if strings.HasPrefix(opts.Address, "redis://") {
		parsed, err := url.Parse(opts.Address)
		if err != nil {
			return nil, fmt.Errorf("redisclient: invalid redis url: %w", err)
		}
		if parsed.Host == "" {
			return nil, errors.New("redisclient: redis url must include host")
		}
		address = parsed.Host
		if parsed.User != nil {
			password, _ = parsed.User.Password()
		}
		if parsed.Path != "" && parsed.Path != "/" {
			val := strings.TrimPrefix(parsed.Path, "/")
			if val != "" {
				if db, err := strconv.Atoi(val); err == nil {
					database = db
				}
			}
		}
	}

	return &Client{
		addr:     address,
		password: password,
		database: database,
		timeout:  opts.Timeout,
	}, nil
}

// Do sends a command to Redis and returns the parsed reply.
func (c *Client) Do(ctx context.Context, args ...string) (interface{}, error) {
	if len(args) == 0 {
		return nil, errors.New("redisclient: no command specified")
	}

	d := net.Dialer{}
	if c.timeout > 0 {
		d.Timeout = c.timeout
	}
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	deadline := time.Time{}
	if c.timeout > 0 {
		deadline = time.Now().Add(c.timeout)
	}
	if ctxDeadline, ok := ctx.Deadline(); ok {
		if deadline.IsZero() || ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
	}
	if !deadline.IsZero() {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}

	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	if c.password != "" {
		if err := writeCommand(writer, "AUTH", c.password); err != nil {
			return nil, err
		}
		if err := writer.Flush(); err != nil {
			return nil, err
		}
		if _, err := readReply(reader); err != nil {
			return nil, err
		}
	}
	if c.database > 0 {
		if err := writeCommand(writer, "SELECT", strconv.Itoa(c.database)); err != nil {
			return nil, err
		}
		if err := writer.Flush(); err != nil {
			return nil, err
		}
		if _, err := readReply(reader); err != nil {
			return nil, err
		}
	}

	if err := writeCommand(writer, args...); err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	return readReply(reader)
}

func writeCommand(w *bufio.Writer, args ...string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return nil
}

func readReply(r *bufio.Reader) (interface{}, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+':
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		return line, nil
	case '-':
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		return nil, errors.New(line)
	case ':':
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		val, convErr := strconv.ParseInt(line, 10, 64)
		if convErr != nil {
			return nil, convErr
		}
		return val, nil
	case '$':
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		length, convErr := strconv.Atoi(line)
		if convErr != nil {
			return nil, convErr
		}
		if length == -1 {
			return nil, nil
		}
		buf := make([]byte, length)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		if _, err := r.Discard(2); err != nil { // discard CRLF
			return nil, err
		}
		return buf, nil
	default:
		return nil, fmt.Errorf("redisclient: unsupported reply prefix %q", prefix)
	}
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}
