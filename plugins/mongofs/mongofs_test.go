package mongofs

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

var (
	oidA = objectID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	oidB = objectID{0xAA, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 0xFF}
)

// fakeServer is a MongoDB that knows the commands the panel sends.
type fakeServer struct {
	addr     string
	user     string
	password string
	cursors  int // getMore calls served
}

func (f *fakeServer) docs(coll string) []bsonD {
	switch coll {
	case "orders":
		return []bsonD{
			{{"_id", oidA}, {"total", 12.5}, {"paid", true}, {"note", "a <b> & c"},
				{"when", time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.UTC)},
				{"lines", []any{int32(1), int64(1) << 40, nil}}, {"meta", bsonD{{"k", "v"}}},
				{"blob", bsonBinary{Subtype: 0, Data: []byte("hi")}}, {"ts", bsonTimestamp{T: 7, I: 1}}},
			{{"_id", "abc def"}, {"total", int32(3)}},
			{{"_id", int32(42)}},
			{{"_id", oidB}, {"empty", bsonD{}}, {"list", []any{}}},
		}
	case "paged":
		var out []bsonD
		for i := 0; i < 5; i++ {
			out = append(out, bsonD{{"_id", int32(i)}})
		}
		return out
	case "big":
		out := make([]bsonD, 1200)
		for i := range out {
			out[i] = bsonD{{"_id", int64(i)}}
		}
		return out
	}
	return nil
}

func startFake(t *testing.T, user, password string) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeServer{addr: ln.Addr().String(), user: user, password: password}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeServer) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	authed := f.user == ""
	var scram struct{ firstBare, serverFirst string }
	for {
		var head [16]byte
		if _, err := io.ReadFull(c, head[:]); err != nil {
			return
		}
		size := int(binary.LittleEndian.Uint32(head[:4]))
		rest := make([]byte, size-16)
		if _, err := io.ReadFull(c, rest); err != nil {
			return
		}
		cmd, err := decodeDoc(rest[5:])
		if err != nil {
			return
		}
		reply := f.handle(cmd, &authed, &scram)
		body, _ := reply.encode()
		msg := appendInt32(nil, int32(21+len(body)))
		msg = appendInt32(msg, 1)
		msg = appendInt32(msg, int32(binary.LittleEndian.Uint32(head[4:])))
		msg = appendInt32(msg, opMsg)
		msg = appendInt32(msg, 0)
		msg = append(msg, 0)
		msg = append(msg, body...)
		if _, err := c.Write(msg); err != nil {
			return
		}
	}
}

func fail(text string) bsonD {
	return bsonD{{"ok", float64(0)}, {"errmsg", text}}
}

func cursorReply(ns string, id int64, key string, docs []bsonD) bsonD {
	items := make([]any, len(docs))
	for i, d := range docs {
		items[i] = d
	}
	return bsonD{{"cursor", bsonD{{key, items}, {"id", id}, {"ns", ns}}}, {"ok", float64(1)}}
}

func (f *fakeServer) handle(cmd bsonD, authed *bool, scram *struct{ firstBare, serverFirst string }) bsonD {
	name := cmd[0].Key
	db, _ := cmd.get("$db").(string)
	switch name {
	case "isMaster":
		return bsonD{{"ismaster", true}, {"ok", float64(1)}}
	case "saslStart":
		payload := string(cmd.get("payload").(bsonBinary).Data)
		scram.firstBare = strings.TrimPrefix(payload, "n,,")
		nonce := parseSCRAM(scram.firstBare)["r"]
		salt := []byte("saltsalt")
		scram.serverFirst = "r=" + nonce + "srv,s=" + base64.StdEncoding.EncodeToString(salt) + ",i=4096"
		return bsonD{{"conversationId", int32(1)}, {"done", false}, {"payload", []byte(scram.serverFirst)}, {"ok", float64(1)}}
	case "saslContinue":
		payload := string(cmd.get("payload").(bsonBinary).Data)
		fields := parseSCRAM(payload)
		salted, _ := pbkdf2.Key(sha256.New, f.password, []byte("saltsalt"), 4096, 32)
		noProof := payload[:strings.LastIndex(payload, ",p=")]
		authMessage := scram.firstBare + "," + scram.serverFirst + "," + noProof
		clientKey := mac(salted, "Client Key")
		stored := sha256.Sum256(clientKey)
		sig := mac(stored[:], authMessage)
		proof, _ := base64.StdEncoding.DecodeString(fields["p"])
		for i := range proof {
			proof[i] ^= sig[i]
		}
		if string(proof) != string(clientKey) {
			return fail("Authentication failed.")
		}
		*authed = true
		v := base64.StdEncoding.EncodeToString(mac(mac(salted, "Server Key"), authMessage))
		return bsonD{{"conversationId", int32(1)}, {"done", true}, {"payload", []byte("v=" + v)}, {"ok", float64(1)}}
	}
	if !*authed {
		return fail("command " + name + " requires authentication")
	}
	switch name {
	case "listDatabases":
		return bsonD{{"databases", []any{bsonD{{"name", "admin"}}, bsonD{{"name", "shop"}}}}, {"ok", float64(1)}}
	case "listCollections":
		if db != "shop" {
			return cursorReply(db+".$cmd.listCollections", 0, "firstBatch", nil)
		}
		var docs []bsonD
		for _, n := range []string{"orders", "paged", "big", "system.views"} {
			docs = append(docs, bsonD{{"name", n}})
		}
		return cursorReply("shop.$cmd.listCollections", 0, "firstBatch", docs)
	case "find":
		coll, _ := cmd[0].Value.(string)
		all := f.docs(coll)
		if filter, ok := cmd.get("filter").(bsonD); ok {
			var hit []bsonD
			for _, d := range all {
				if fmt.Sprint(d.get("_id")) == fmt.Sprint(filter.get("_id")) ||
					toJSON(d.get("_id"), "") == toJSON(filter.get("_id"), "") {
					hit = append(hit, d)
				}
			}
			return cursorReply("shop."+coll, 0, "firstBatch", hit)
		}
		limit := len(all)
		if l, ok := numberOf(cmd.get("limit")); ok && int(l) < limit {
			limit = int(l)
		}
		all = all[:limit]
		if coll == "paged" {
			return cursorReply("shop.paged", 77, "firstBatch", all[:2])
		}
		if _, ok := cmd.get("projection").(bsonD); ok {
			for i, d := range all {
				all[i] = bsonD{{"_id", d.get("_id")}}
			}
		}
		return cursorReply("shop."+coll, 0, "firstBatch", all)
	case "getMore":
		f.cursors++
		all := f.docs("paged")
		from := 2 * f.cursors
		to := min(from+2, len(all))
		id := int64(77)
		if to >= len(all) {
			id = 0
		}
		return cursorReply("shop.paged", id, "nextBatch", all[from:to])
	case "killCursors":
		return bsonD{{"ok", float64(1)}}
	}
	return fail("no such command: " + name)
}

func connectTo(f *fakeServer, user, pass string) func(context.Context) (*conn, error) {
	return func(ctx context.Context) (*conn, error) {
		return dial(ctx, connConfig{addr: f.addr, user: user, pass: pass, authSource: "admin"})
	}
}

func names(t *testing.T, v *mongoVFS, p string) ([]string, error) {
	t.Helper()
	var out []string
	err := v.ReadDir(context.Background(), p, func(items []vfs.VFSItem) {
		for _, it := range items {
			out = append(out, it.Name)
		}
	})
	return out, err
}

func TestMongoVFSBrowses(t *testing.T) {
	f := startFake(t, "", "")
	v := newMongoVFS(connectTo(f, "", ""))
	defer func() { _ = v.Close() }()
	ctx := context.Background()

	dbs, err := names(t, v, "/")
	if err != nil || strings.Join(dbs, ",") != "admin,shop" {
		t.Fatalf("databases %v, %v", dbs, err)
	}
	colls, err := names(t, v, "/shop")
	if err != nil || len(colls) != 4 || colls[0] != "orders" {
		t.Fatalf("collections %v, %v", colls, err)
	}
	docs, err := names(t, v, "/shop/orders")
	want := oidA.hex() + ".json,s_abc%20def.json,i_42.json," + oidB.hex() + ".json"
	if err != nil || strings.Join(docs, ",") != want {
		t.Fatalf("documents %v, %v (want %s)", docs, err, want)
	}

	f2, err := v.Open(ctx, "/shop/orders/"+oidA.hex()+".json")
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, f2.Size())
	_, _ = f2.ReadAt(ctx, data, 0)
	_ = f2.Close()
	text := string(data)
	for _, frag := range []string{
		`"_id": {"$oid": "` + oidA.hex() + `"}`, `"total": 12.5`, `"paid": true`, `"note": "a <b> & c"`,
		`"$date": "2026-01-02T03:04:05.006Z"`, `1099511627776`, `null`, `"k": "v"`,
		`"base64": "aGk="`, `"$timestamp": {"t": 7, "i": 1}`,
	} {
		if !strings.Contains(text, frag) {
			t.Errorf("document lacks %q:\n%s", frag, text)
		}
	}
	if !strings.HasSuffix(text, "}\n") {
		t.Errorf("document should end with a newline: %q", text[len(text)-3:])
	}

	// A string id and an integer id are found from their names.
	for _, name := range []string{"s_abc%20def.json", "i_42.json"} {
		if _, err := v.Stat(ctx, "/shop/orders/"+name); err != nil {
			t.Errorf("Stat(%s): %v", name, err)
		}
	}
	// A name never listed is found from its form; one that says nothing is not.
	v.mu.Lock()
	v.ids = map[string]any{}
	v.mu.Unlock()
	if it, err := v.Stat(ctx, "/shop/orders/"+oidA.hex()+".json"); err != nil || !it.SizeKnown || it.Size < 50 {
		t.Fatalf("Stat by name only: %+v, %v", it, err)
	}
	if _, err := v.Stat(ctx, "/shop/orders/bogus.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bogus name: %v", err)
	}
	if _, err := v.Stat(ctx, "/shop/orders/i_7.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing id: %v", err)
	}
	if _, err := v.Open(ctx, "/shop/orders"); !errors.Is(err, errIsADirectory) {
		t.Fatalf("Open on a collection: %v", err)
	}
	if err := v.ReadDir(ctx, "/shop/orders/x.json", nil); !errors.Is(err, errNotADirectory) {
		t.Fatalf("ReadDir on a document: %v", err)
	}
}

func TestMongoVFSPagingAndTruncation(t *testing.T) {
	f := startFake(t, "", "")
	v := newMongoVFS(connectTo(f, "", ""))
	defer func() { _ = v.Close() }()

	docs, err := names(t, v, "/shop/paged")
	if err != nil || len(docs) != 5 {
		t.Fatalf("a paged collection: %v, %v", docs, err)
	}
	big, err := names(t, v, "/shop/big")
	if len(big) != maxDocs || !errors.Is(err, truncatedError{}) {
		t.Fatalf("a big collection: %d names, %v", len(big), err)
	}
	if !strings.HasPrefix(big[0], "i_0") {
		t.Fatalf("an int64 id should be named i_N: %q", big[0])
	}
}

func TestMongoVFSStatSetPathAndReadOnly(t *testing.T) {
	f := startFake(t, "", "")
	v := newMongoVFS(connectTo(f, "", ""))
	defer func() { _ = v.Close() }()
	ctx := context.Background()

	if err := v.SetPath("/shop/orders"); err != nil {
		t.Fatal(err)
	}
	if v.GetPath() != "/shop/orders" || v.IsAtRoot() || v.PanelTitle(v.GetPath()) != "MongoDB:shop/orders" || v.PanelTitle("/") != "MongoDB" {
		t.Fatalf("path %q", v.GetPath())
	}
	for _, p := range []string{"/", "/shop", "/shop/orders"} {
		if it, err := v.Stat(ctx, p); err != nil || !it.IsDir {
			t.Errorf("Stat(%q) = %+v, %v", p, it, err)
		}
	}
	for _, p := range []string{"/nodb", "/shop/nocoll"} {
		if _, err := v.Stat(ctx, p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Stat(%q): %v", p, err)
		}
	}
	if err := v.SetPath("/shop/orders/i_42.json"); !errors.Is(err, errNotADirectory) {
		t.Fatalf("SetPath on a document: %v", err)
	}
	if v.Clone().(*mongoVFS).GetPath() != "/shop/orders" {
		t.Fatal("clone lost the path")
	}
	_, createErr := v.Create(ctx, "/x")
	for i, err := range []error{v.MkDir(ctx, "/x"), v.Remove(ctx, "/x"), v.Rename(ctx, "/x", "/y"), v.SetAttributes(ctx, "/x", vfs.VFSItem{}), createErr} {
		if !errors.Is(err, os.ErrPermission) || err.Error() == "" {
			t.Errorf("mutation %d: %v", i, err)
		}
	}
	_ = v.Close()
	if _, err := v.databases(ctx); err == nil {
		t.Fatal("a closed panel should not reconnect")
	}
}

func TestSCRAMAuthentication(t *testing.T) {
	f := startFake(t, "user=,x", "pässword")
	v := newMongoVFS(connectTo(f, "user=,x", "pässword"))
	if dbs, err := v.databases(context.Background()); err != nil || len(dbs) != 2 {
		t.Fatalf("with the right password: %v, %v", dbs, err)
	}
	_ = v.Close()

	bad := newMongoVFS(connectTo(f, "user=,x", "wrong"))
	if _, err := bad.databases(context.Background()); !errors.Is(err, errAuth) {
		t.Fatalf("with a wrong password: %v", err)
	}
	none := newMongoVFS(connectTo(f, "", ""))
	if _, err := none.databases(context.Background()); err == nil || !strings.Contains(err.Error(), "authentication") {
		t.Fatalf("without credentials: %v", err)
	}
}

func TestConnectionFailures(t *testing.T) {
	dead := newMongoVFS(func(ctx context.Context) (*conn, error) {
		return dial(ctx, connConfig{addr: "127.0.0.1:1"})
	})
	if _, err := dead.databases(context.Background()); err == nil || !strings.Contains(err.Error(), "MongoDB") {
		t.Fatalf("an unreachable server: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := startFake(t, "", "")
	if _, err := newMongoVFS(connectTo(f, "", "")).databases(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled context: %v", err)
	}
	// A server that hangs up mid-conversation drops the connection, and the
	// next call connects again.
	v := newMongoVFS(connectTo(f, "", ""))
	if _, err := v.databases(context.Background()); err != nil {
		t.Fatal(err)
	}
	v.mu.Lock()
	v.c.close()
	v.mu.Unlock()
	if _, err := v.databases(context.Background()); err == nil {
		t.Fatal("a dead connection should fail once")
	}
	if _, err := v.databases(context.Background()); err != nil {
		t.Fatalf("and then reconnect: %v", err)
	}
	if _, err := v.run(context.Background(), "admin", bsonD{{"nonsense", int32(1)}}); err == nil || !strings.HasPrefix(err.Error(), "mongodb: ") {
		t.Fatalf("a refused command: %v", err)
	}
}

func TestParseURI(t *testing.T) {
	cfg, err := parseURI("mongodb://bob:p%40ss@db1.example:27018,db2/shop?tls=true")
	if err != nil || cfg.addr != "db1.example:27018" || cfg.user != "bob" || cfg.pass != "p@ss" || cfg.authSource != "shop" || !cfg.useTLS {
		t.Fatalf("%+v, %v", cfg, err)
	}
	cfg, err = parseURI("mongodb://localhost/?authSource=users")
	if err != nil || cfg.addr != "localhost:27017" || cfg.authSource != "users" || cfg.useTLS {
		t.Fatalf("%+v, %v", cfg, err)
	}
	if cfg, _ := parseURI("mongodb://[::1]"); cfg.addr != "[::1]:27017" {
		t.Fatalf("ipv6: %+v", cfg)
	}
	for _, bad := range []string{"", "http://x", "mongodb+srv://x", "mongodb://", "mongodb://h/?authMechanism=PLAIN"} {
		if _, err := parseURI(bad); !errors.Is(err, errURI) {
			t.Errorf("parseURI(%q) = %v", bad, err)
		}
	}
}

func TestBSONRoundTripAndErrors(t *testing.T) {
	in := bsonD{{"s", "x"}, {"i", 5}, {"big", int64(1) << 40}, {"i32", int32(-2)}, {"i64", int64(-3)}, {"f", 1.5}, {"b", true}, {"n", nil},
		{"o", oidA}, {"bin", []byte{1, 2}}, {"doc", bsonD{{"a", int32(1)}}}, {"arr", []any{"p", int32(2)}}}
	raw, err := in.encode()
	if err != nil {
		t.Fatal(err)
	}
	out, err := decodeDoc(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := toJSON(out, ""); !strings.Contains(got, `"big": 1099511627776`) || !strings.Contains(got, `"i": 5`) || !strings.Contains(got, `"arr": ["p",2]`) {
		t.Fatalf("round trip: %s", got)
	}
	if _, err := (bsonD{{"x", struct{}{}}}).encode(); err == nil {
		t.Fatal("an unsupported Go type should not encode")
	}
	for _, bad := range [][]byte{nil, {1, 2, 3}, {9, 0, 0, 0, 0}, append(append([]byte{}, raw[:len(raw)-1]...), 1), {12, 0, 0, 0, 0x7F, 'a', 0, 0, 0, 0, 0, 0}} {
		if _, err := decodeDoc(bad); !errors.Is(err, errBSON) {
			t.Errorf("decodeDoc(%v) = %v", bad, err)
		}
	}
	if got := toJSON(bsonD{{"d", bsonRaw{Type: tDecimal, Data: []byte{1}}}, {"nan", nanValue()}, {"empty", bsonD{}}}, ""); !strings.Contains(got, "$unsupported") || !strings.Contains(got, "$numberDouble") {
		t.Fatalf("odd values: %s", got)
	}
}

func nanValue() float64 {
	zero := 0.0
	return zero / zero
}

func TestPluginAndNaming(t *testing.T) {
	p := NewPlugin()
	if p.GetName() != "MongoDB" || p.Close() != nil || p.Init(nil) == nil {
		t.Fatal("plugin identity")
	}
	t.Setenv("MONGODB_URI", "mongodb+srv://x")
	if _, err := connectFromEnv(context.Background()); !errors.Is(err, errURI) {
		t.Fatalf("connectFromEnv: %v", err)
	}
	t.Setenv("MONGODB_URI", "")
	if _, err := connectFromEnv(context.Background()); err == nil {
		t.Skip("a MongoDB is listening on the default port here")
	}
	for id, want := range map[any]string{oidA: oidA.hex() + ".json", "a/b": "s_a%2Fb.json", int32(5): "i_5.json", int64(6): "i_6.json"} {
		if got := docFileName(id); got != want {
			t.Errorf("docFileName(%v) = %q, want %q", id, got, want)
		}
		if back, ok := idFromName(want); !ok || toJSON(back, "") == "" {
			t.Errorf("idFromName(%q) = %v, %v", want, back, ok)
		}
	}
	if got := docFileName(bsonD{{"a", int32(1)}}); !strings.HasPrefix(got, "j_") {
		t.Errorf("a compound id: %q", got)
	}
	if _, ok := idFromName("j_x.json"); ok {
		t.Error("a compound id cannot be read back from its name")
	}
	if _, ok := idFromName("plain"); ok {
		t.Error("a name without .json is not a document")
	}
	if loc := parseLocation("/a/b/c.json"); loc.db != "a" || loc.coll != "b" || loc.doc != "c.json" || loc.depth != 3 {
		t.Errorf("location %+v", loc)
	}
}
