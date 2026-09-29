package mongofs

import (
	"context"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The wire protocol is OP_MSG (MongoDB 3.6 and later): one header, a flags
// word and one section holding the command document. Only what the panel
// needs is here: no pooling, no replica-set discovery, no compression.
const (
	opMsg          = 2013
	maxMessageSize = 48 << 20
	defaultPort    = "27017"
	dialTimeout    = 10 * time.Second
	ioTimeout      = 60 * time.Second
)

var (
	errURI  = errors.New("mongofs: unsupported connection string")
	errAuth = errors.New("mongofs: authentication failed")
)

// connConfig is what a connection string says.
type connConfig struct {
	addr       string
	user, pass string
	authSource string
	useTLS     bool
}

// parseURI reads mongodb://[user:pass@]host[:port][/db][?authSource=x&tls=true].
// SRV strings and replica-set seed lists are not supported: the first host of
// a list is used.
func parseURI(raw string) (connConfig, error) {
	rest, ok := strings.CutPrefix(raw, "mongodb://")
	if !ok {
		return connConfig{}, fmt.Errorf("%w: use mongodb://host[:port]", errURI)
	}
	// A seed list ("h1:27017,h2:27017") is not a valid URL host, so the first
	// host is cut out by hand before the rest is parsed.
	authority, tail := rest, ""
	if i := strings.IndexAny(rest, "/?"); i >= 0 {
		authority, tail = rest[:i], rest[i:]
	}
	userinfo, hosts := "", authority
	if i := strings.LastIndex(authority, "@"); i >= 0 {
		userinfo, hosts = authority[:i+1], authority[i+1:]
	}
	host, _, _ := strings.Cut(hosts, ",")
	u, err := url.Parse("mongodb://" + userinfo + host + tail)
	if err != nil || u.Host == "" {
		return connConfig{}, fmt.Errorf("%w: use mongodb://host[:port]", errURI)
	}
	if _, _, err := net.SplitHostPort(u.Host); err != nil {
		host = net.JoinHostPort(strings.Trim(u.Host, "[]"), defaultPort)
	} else {
		host = u.Host
	}
	cfg := connConfig{addr: host, authSource: "admin"}
	if u.User != nil {
		cfg.user = u.User.Username()
		cfg.pass, _ = u.User.Password()
	}
	q := u.Query()
	if v := q.Get("authSource"); v != "" {
		cfg.authSource = v
	} else if db := strings.TrimPrefix(u.Path, "/"); db != "" && cfg.user != "" {
		cfg.authSource = db
	}
	cfg.useTLS = q.Get("tls") == "true" || q.Get("ssl") == "true"
	if mech := q.Get("authMechanism"); mech != "" && mech != "SCRAM-SHA-256" {
		return connConfig{}, fmt.Errorf("%w: only SCRAM-SHA-256 authentication is supported", errURI)
	}
	return cfg, nil
}

// conn is one connection. It is used by one goroutine at a time.
type conn struct {
	c     net.Conn
	reqID int32
}

func dial(ctx context.Context, cfg connConfig) (*conn, error) {
	d := net.Dialer{Timeout: dialTimeout}
	raw, err := d.DialContext(ctx, "tcp", cfg.addr)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s: %w", mongoText("Mongo.Unreachable",
			"Cannot reach the MongoDB server (check MONGODB_URI and that it is running)",
			"Нет связи с сервером MongoDB (проверьте MONGODB_URI и что сервер запущен)"), err)
	}
	if cfg.useTLS {
		host, _, _ := net.SplitHostPort(cfg.addr)
		tc := tls.Client(raw, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, err
		}
		raw = tc
	}
	c := &conn{c: raw}
	if _, err := c.command(ctx, "admin", bsonD{{"isMaster", int32(1)}}); err != nil {
		_ = raw.Close()
		return nil, err
	}
	if cfg.user != "" {
		if err := c.authenticate(ctx, cfg); err != nil {
			_ = raw.Close()
			return nil, err
		}
	}
	return c, nil
}

func (c *conn) close() {
	if c != nil {
		_ = c.c.Close()
	}
}

// command runs one command against a database and returns the reply document
// if the server says ok.
func (c *conn) command(ctx context.Context, db string, cmd bsonD) (bsonD, error) {
	doc := append(append(bsonD{}, cmd...), bsonE{"$db", db})
	body, err := doc.encode()
	if err != nil {
		return nil, err
	}
	c.reqID++
	msg := make([]byte, 0, 21+len(body))
	msg = appendInt32(msg, int32(21+len(body))) // #nosec G115 -- a command is small
	msg = appendInt32(msg, c.reqID)
	msg = appendInt32(msg, 0)
	msg = appendInt32(msg, opMsg)
	msg = appendInt32(msg, 0) // flag bits
	msg = append(msg, 0)      // section kind 0: one document
	msg = append(msg, body...)

	deadline := time.Now().Add(ioTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = c.c.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = c.c.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	if _, err := c.c.Write(msg); err != nil {
		return nil, ctxOr(ctx, err)
	}
	reply, err := readReply(c.c)
	if err != nil {
		return nil, ctxOr(ctx, err)
	}
	if ok, _ := numberOf(reply.get("ok")); ok != 1 {
		text, _ := reply.get("errmsg").(string)
		if text == "" {
			text = "the server refused the command"
		}
		return nil, fmt.Errorf("mongodb: %s", text)
	}
	return reply, nil
}

func ctxOr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func readReply(r io.Reader) (bsonD, error) {
	var head [16]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, err
	}
	size := int(binary.LittleEndian.Uint32(head[:4]))
	if size < 21 || size > maxMessageSize {
		return nil, fmt.Errorf("mongodb: bad message length %d", size)
	}
	if op := binary.LittleEndian.Uint32(head[12:]); op != opMsg {
		return nil, fmt.Errorf("mongodb: unexpected opcode %d", op)
	}
	rest := make([]byte, size-16)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, err
	}
	flags := binary.LittleEndian.Uint32(rest)
	body := rest[4:]
	if flags&1 != 0 && len(body) >= 4 { // checksum present
		body = body[:len(body)-4]
	}
	if len(body) < 1 || body[0] != 0 {
		return nil, errors.New("mongodb: unsupported reply section")
	}
	return decodeDoc(body[1:])
}

// numberOf reads a BSON number of any width.
func numberOf(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func mac(key []byte, msg string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(msg))
	return h.Sum(nil)
}

// authenticate runs SCRAM-SHA-256 (RFC 5802 / 7677 as MongoDB uses it).
func (c *conn) authenticate(ctx context.Context, cfg connConfig) error {
	nonceRaw := make([]byte, 24)
	if _, err := rand.Read(nonceRaw); err != nil {
		return err
	}
	nonce := base64.StdEncoding.EncodeToString(nonceRaw)
	user := strings.NewReplacer("=", "=3D", ",", "=2C").Replace(cfg.user)
	firstBare := "n=" + user + ",r=" + nonce

	start, err := c.command(ctx, cfg.authSource, bsonD{
		{"saslStart", int32(1)}, {"mechanism", "SCRAM-SHA-256"},
		{"payload", []byte("n,," + firstBare)}, {"autoAuthorize", int32(1)},
		{"options", bsonD{{"skipEmptyExchange", true}}},
	})
	if err != nil {
		return fmt.Errorf("%w: %v", errAuth, err)
	}
	serverFirst := payloadString(start)
	fields := parseSCRAM(serverFirst)
	salt, err := base64.StdEncoding.DecodeString(fields["s"])
	iter, iterErr := strconv.Atoi(fields["i"])
	if err != nil || iterErr != nil || iter < 1 || !strings.HasPrefix(fields["r"], nonce) {
		return fmt.Errorf("%w: bad server challenge", errAuth)
	}
	salted, err := pbkdf2.Key(sha256.New, cfg.pass, salt, iter, sha256.Size)
	if err != nil {
		return err
	}
	clientKey := mac(salted, "Client Key")
	stored := sha256.Sum256(clientKey)
	finalNoProof := "c=biws,r=" + fields["r"]
	authMessage := firstBare + "," + serverFirst + "," + finalNoProof
	sig := mac(stored[:], authMessage)
	proof := make([]byte, len(clientKey))
	for i := range proof {
		proof[i] = clientKey[i] ^ sig[i]
	}
	final := finalNoProof + ",p=" + base64.StdEncoding.EncodeToString(proof)

	conversation := start.get("conversationId")
	reply, err := c.command(ctx, cfg.authSource, bsonD{
		{"saslContinue", int32(1)}, {"conversationId", conversation}, {"payload", []byte(final)},
	})
	if err != nil {
		return fmt.Errorf("%w: %v", errAuth, err)
	}
	serverSig := parseSCRAM(payloadString(reply))["v"]
	want := base64.StdEncoding.EncodeToString(mac(mac(salted, "Server Key"), authMessage))
	if subtle.ConstantTimeCompare([]byte(serverSig), []byte(want)) != 1 {
		return fmt.Errorf("%w: the server did not prove it knows the password", errAuth)
	}
	if done, _ := reply.get("done").(bool); !done {
		_, err = c.command(ctx, cfg.authSource, bsonD{
			{"saslContinue", int32(1)}, {"conversationId", conversation}, {"payload", []byte{}},
		})
		return err
	}
	return nil
}

func payloadString(d bsonD) string {
	if b, ok := d.get("payload").(bsonBinary); ok {
		return string(b.Data)
	}
	return ""
}

func parseSCRAM(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		if k, v, ok := strings.Cut(part, "="); ok {
			out[k] = v
		}
	}
	return out
}
