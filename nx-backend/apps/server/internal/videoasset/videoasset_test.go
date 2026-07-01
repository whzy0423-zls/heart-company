package videoasset

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

func TestCreateUsesUploadAssetObjectURLWhenAvailable(t *testing.T) {
	state := &videoAssetTestState{
		uploadAsset: uploadasset.Asset{
			ID:        7,
			Key:       "upload-assets/7",
			Name:      "demo.mp4",
			ObjectKey: "uploads/video/generated/demo.mp4",
			ObjectURL: "https://cdn.example.com/uploads/video/generated/demo.mp4",
			Data:      []byte("demo"),
		},
	}
	db := openVideoAssetTestDB(t, state)
	defer db.Close()

	store := NewStore(db, uploadasset.NewStore(db))
	asset, err := store.Create(context.Background(), CreateInput{
		AssetID: "7",
		Name:    "demo",
		Type:    "video",
	})
	if err != nil {
		t.Fatal(err)
	}
	if asset.URL != "https://cdn.example.com/uploads/video/generated/demo.mp4" {
		t.Fatalf("expected object url to be stored, got %q", asset.URL)
	}
	if asset.CoverURL != asset.URL {
		t.Fatalf("expected cover url to match object url, got %q", asset.CoverURL)
	}
}

func TestCreateRejectsUploadAssetWithoutPublicObjectURL(t *testing.T) {
	state := &videoAssetTestState{
		uploadAsset: uploadasset.Asset{
			ID:   7,
			Key:  "upload-assets/7",
			Name: "demo.mp4",
			Data: []byte("demo"),
		},
	}
	db := openVideoAssetTestDB(t, state)
	defer db.Close()

	store := NewStore(db, uploadasset.NewStore(db))
	_, err := store.Create(context.Background(), CreateInput{
		AssetID: "7",
		Name:    "demo",
		Type:    "video",
		URL:     "/api/upload-assets/7",
	})
	if err == nil {
		t.Fatal("expected upload asset without public object url to be rejected")
	}
	if !strings.Contains(err.Error(), "公网") {
		t.Fatalf("expected public url error, got %q", err.Error())
	}
	if len(state.inserted) != 0 {
		t.Fatalf("expected no video asset insert, got %d", len(state.inserted))
	}
}

func TestCreateRejectsUploadAssetWithLocalObjectURL(t *testing.T) {
	state := &videoAssetTestState{
		uploadAsset: uploadasset.Asset{
			ID:        7,
			Key:       "upload-assets/7",
			Name:      "demo.mp4",
			ObjectURL: "/api/uploads/video/demo.mp4",
			Data:      []byte("demo"),
		},
	}
	db := openVideoAssetTestDB(t, state)
	defer db.Close()

	store := NewStore(db, uploadasset.NewStore(db))
	_, err := store.Create(context.Background(), CreateInput{
		AssetID: "7",
		Name:    "demo",
		Type:    "video",
	})
	if err == nil {
		t.Fatal("expected local object url to be rejected")
	}
	if !strings.Contains(err.Error(), "公网") {
		t.Fatalf("expected public url error, got %q", err.Error())
	}
	if len(state.inserted) != 0 {
		t.Fatalf("expected no video asset insert, got %d", len(state.inserted))
	}
}

func TestCreateAllowsDirectPublicURLWithoutUploadAsset(t *testing.T) {
	state := &videoAssetTestState{}
	db := openVideoAssetTestDB(t, state)
	defer db.Close()

	store := NewStore(db, uploadasset.NewStore(db))
	asset, err := store.Create(context.Background(), CreateInput{
		Name: "external",
		Type: "video",
		URL:  "https://cdn.example.com/external.mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if asset.URL != "https://cdn.example.com/external.mp4" {
		t.Fatalf("expected direct public url to be stored, got %q", asset.URL)
	}
}

func TestIsPublicHTTPURLRejectsPrivateAndLocalHosts(t *testing.T) {
	for _, raw := range []string{
		"http://localhost/a.mp4",
		"http://127.0.0.1/a.mp4",
		"http://10.0.0.2/a.mp4",
		"http://172.31.0.2/a.mp4",
		"http://192.168.1.2/a.mp4",
		"http://169.254.1.2/a.mp4",
	} {
		if isPublicHTTPURL(raw) {
			t.Fatalf("expected %s to be rejected as non-public", raw)
		}
	}
	if !isPublicHTTPURL("https://cdn.example.com/a.mp4") {
		t.Fatal("expected public CDN URL to be accepted")
	}
}

type videoAssetTestState struct {
	inserted    []videoAssetRecord
	uploadAsset uploadasset.Asset
	seq         int64
}

type videoAssetRecord struct {
	assetID string
	cover   string
	name    string
	remark  string
	typeVal string
	url     string
}

type videoAssetTestDriver struct{}

type videoAssetTestConn struct{ state *videoAssetTestState }

type videoAssetTestRows struct {
	columns []string
	values  []driver.Value
	read    bool
}

var (
	videoAssetTestMu     sync.Mutex
	videoAssetTestStates = map[string]*videoAssetTestState{}
)

func init() {
	sql.Register("videoasset-test", videoAssetTestDriver{})
}

func openVideoAssetTestDB(t *testing.T, state *videoAssetTestState) *sql.DB {
	t.Helper()
	name := t.Name()
	videoAssetTestMu.Lock()
	videoAssetTestStates[name] = state
	videoAssetTestMu.Unlock()
	t.Cleanup(func() {
		videoAssetTestMu.Lock()
		delete(videoAssetTestStates, name)
		videoAssetTestMu.Unlock()
	})
	db, err := sql.Open("videoasset-test", name)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func (videoAssetTestDriver) Open(name string) (driver.Conn, error) {
	videoAssetTestMu.Lock()
	defer videoAssetTestMu.Unlock()
	state := videoAssetTestStates[name]
	if state == nil {
		return nil, errors.New("missing videoasset test state")
	}
	return &videoAssetTestConn{state: state}, nil
}

func (c *videoAssetTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not supported")
}
func (c *videoAssetTestConn) Close() error              { return nil }
func (c *videoAssetTestConn) Begin() (driver.Tx, error) { return nil, errors.New("not supported") }

func (c *videoAssetTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT nextval(pg_get_serial_sequence('upload_assets','id'))"):
		return &videoAssetTestRows{columns: []string{"nextval"}, values: []driver.Value{int64(7)}}, nil
	case strings.Contains(query, "INSERT INTO upload_assets"):
		if c.state.uploadAsset.ID == 0 {
			c.state.uploadAsset.ID = 7
		}
		c.state.uploadAsset.Key = namedString(args, 2)
		c.state.uploadAsset.Name = namedString(args, 3)
		c.state.uploadAsset.ObjectKey = namedString(args, 8)
		c.state.uploadAsset.ObjectURL = namedString(args, 9)
		c.state.uploadAsset.Data = namedBytes(args, 7)
		return &videoAssetTestRows{
			columns: []string{"id", "key", "name", "content_type", "size", "data", "object_key", "object_url"},
			values:  []driver.Value{c.state.uploadAsset.ID, c.state.uploadAsset.Key, c.state.uploadAsset.Name, namedString(args, 5), namedInt64(args, 6), c.state.uploadAsset.Data, c.state.uploadAsset.ObjectKey, c.state.uploadAsset.ObjectURL},
		}, nil
	case strings.Contains(query, "FROM upload_assets") && strings.Contains(query, "WHERE id=$1"):
		if c.state.uploadAsset.ID == 0 || namedInt64(args, 1) != c.state.uploadAsset.ID {
			return nil, sql.ErrNoRows
		}
		return &videoAssetTestRows{
			columns: []string{"id", "key", "name", "content_type", "size", "data", "object_key", "object_url"},
			values:  []driver.Value{c.state.uploadAsset.ID, c.state.uploadAsset.Key, c.state.uploadAsset.Name, "video/mp4", int64(len(c.state.uploadAsset.Data)), c.state.uploadAsset.Data, c.state.uploadAsset.ObjectKey, c.state.uploadAsset.ObjectURL},
		}, nil
	case strings.Contains(query, "SELECT count(*) FROM video_assets WHERE"):
		return &videoAssetTestRows{columns: []string{"count"}, values: []driver.Value{int64(0)}}, nil
	case strings.Contains(query, "INSERT INTO video_assets"):
		asset := videoAssetRecord{
			assetID: namedString(args, 3),
			cover:   namedString(args, 5),
			name:    namedString(args, 2),
			remark:  namedString(args, 6),
			typeVal: namedString(args, 1),
			url:     namedString(args, 4),
		}
		if asset.assetID == "7" && c.state.uploadAsset.ObjectURL != "" {
			asset.url = c.state.uploadAsset.ObjectURL
			if asset.cover == "" {
				asset.cover = asset.url
			}
		}
		c.state.inserted = append(c.state.inserted, asset)
		return &videoAssetTestRows{columns: []string{"id"}, values: []driver.Value{"42"}}, nil
	case strings.Contains(query, "SELECT id::text, type, name, COALESCE(asset_id::text,''), url, cover_url, remark, status, create_time, update_time"):
		if len(c.state.inserted) == 0 {
			return nil, sql.ErrNoRows
		}
		asset := c.state.inserted[len(c.state.inserted)-1]
		return &videoAssetTestRows{
			columns: []string{"id", "type", "name", "asset_id", "url", "cover_url", "remark", "status", "create_time", "update_time"},
			values:  []driver.Value{"42", asset.typeVal, asset.name, asset.assetID, asset.url, asset.cover, asset.remark, "active", time.Now(), time.Now()},
		}, nil
	default:
		return nil, errors.New("unexpected query: " + query)
	}
}

func (c *videoAssetTestConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return videoAssetTestResult(1), nil
}

func (r *videoAssetTestRows) Columns() []string { return r.columns }
func (r *videoAssetTestRows) Close() error      { return nil }
func (r *videoAssetTestRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	copy(dest, r.values)
	r.read = true
	return nil
}

type videoAssetTestResult int64

func (videoAssetTestResult) LastInsertId() (int64, error)   { return 0, nil }
func (r videoAssetTestResult) RowsAffected() (int64, error) { return int64(r), nil }

func namedString(args []driver.NamedValue, ordinal int) string {
	for _, arg := range args {
		if arg.Ordinal == ordinal {
			if v, ok := arg.Value.(string); ok {
				return v
			}
		}
	}
	return ""
}

func namedInt64(args []driver.NamedValue, ordinal int) int64 {
	for _, arg := range args {
		if arg.Ordinal == ordinal {
			switch v := arg.Value.(type) {
			case int64:
				return v
			case int:
				return int64(v)
			}
		}
	}
	return 0
}

func namedBytes(args []driver.NamedValue, ordinal int) []byte {
	for _, arg := range args {
		if arg.Ordinal == ordinal {
			switch v := arg.Value.(type) {
			case []byte:
				return append([]byte(nil), v...)
			case string:
				return []byte(v)
			}
		}
	}
	return nil
}
