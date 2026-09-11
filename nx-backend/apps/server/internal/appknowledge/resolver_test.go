package appknowledge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestResolveBindingRowsKeepsTheoryAndOnlyCurrentType(t *testing.T) {
	rows := []bindingRow{
		{Layer: LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", LibraryStatus: "enabled", ReleaseID: 100, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(3), LibraryID: 13, LibraryKey: "enneagram-type-03", LibraryStatus: "enabled", ReleaseID: 103, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(2), LibraryID: 12, LibraryKey: "enneagram-type-02", LibraryStatus: "enabled", ReleaseID: 102, ReleaseStatus: "active"},
	}

	resolved := resolveBindingRows(3, nil, rows)
	if resolved.Theory == nil || resolved.Theory.ReleaseID != 100 {
		t.Fatalf("missing theory binding: %+v", resolved)
	}
	if resolved.EnneagramType == nil || resolved.EnneagramType.ReleaseID != 103 || *resolved.EnneagramType.EnneagramType != 3 {
		t.Fatalf("wrong type binding: %+v", resolved)
	}
	if len(resolved.Diagnostics) != 1 || resolved.Diagnostics[0].Code != "cross_type_binding" {
		t.Fatalf("expected cross-type diagnostic, got %+v", resolved.Diagnostics)
	}
}

func TestResolveBindingRowsDegradesWithoutValidMainType(t *testing.T) {
	rows := []bindingRow{{Layer: LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", LibraryStatus: "enabled", ReleaseID: 100, ReleaseStatus: "active"}}
	for _, mainType := range []int{0, -1, 10} {
		resolved := resolveBindingRows(mainType, nil, rows)
		if resolved.Theory == nil || resolved.EnneagramType != nil {
			t.Fatalf("mainType=%d should keep theory only: %+v", mainType, resolved)
		}
	}
}

func TestResolveBindingRowsRejectsDisabledMissingReleaseAndWrongLibrary(t *testing.T) {
	rows := []bindingRow{
		{Layer: LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", LibraryStatus: "disabled", ReleaseID: 100, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(3), LibraryID: 13, LibraryKey: "enneagram-type-02", LibraryStatus: "enabled", ReleaseID: 103, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(3), LibraryID: 14, LibraryKey: "enneagram-type-03", LibraryStatus: "enabled"},
	}

	resolved := resolveBindingRows(3, nil, rows)
	if resolved.Theory != nil || resolved.EnneagramType != nil {
		t.Fatalf("invalid bindings must not resolve: %+v", resolved)
	}
	if len(resolved.Diagnostics) != 3 {
		t.Fatalf("expected three diagnostics, got %+v", resolved.Diagnostics)
	}
}

func TestRequestedTypesNormalizeUniqueValidAndStable(t *testing.T) {
	got := normalizeRequestedTypes([]int{4, 1, 4, 0, 9, 10, 2, -1, 1})
	if !reflect.DeepEqual(got, []int{1, 2, 4, 9}) {
		t.Fatalf("normalizeRequestedTypes = %v, want [1 2 4 9]", got)
	}
}

func TestResolveBindingRowsUsesOnlyRequestedTypesInNumericOrder(t *testing.T) {
	rows := []bindingRow{
		{Layer: LayerEnneagramType, EnneagramType: intPointer(6), LibraryID: 16, LibraryKey: "enneagram-type-06", LibraryStatus: "enabled", ReleaseID: 106, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(4), LibraryID: 14, LibraryKey: "enneagram-type-04", LibraryStatus: "enabled", ReleaseID: 104, ReleaseStatus: "active"},
		{Layer: LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", LibraryStatus: "enabled", ReleaseID: 100, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(1), LibraryID: 11, LibraryKey: "enneagram-type-01", LibraryStatus: "enabled", ReleaseID: 101, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(2), LibraryID: 12, LibraryKey: "enneagram-type-02", LibraryStatus: "enabled", ReleaseID: 102, ReleaseStatus: "active"},
	}

	resolved := resolveBindingRows(6, []int{4, 1, 4, 2, 0, 10}, rows)
	if resolved.EnneagramType != nil {
		t.Fatalf("explicit request must not inject current-card binding: %+v", resolved.EnneagramType)
	}
	got := make([]int, 0, len(resolved.RequestedTypeBindings))
	for _, binding := range resolved.RequestedTypeBindings {
		got = append(got, *binding.EnneagramType)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 4}) {
		t.Fatalf("requested bindings = %v, want [1 2 4]", got)
	}
	for _, binding := range resolved.RequestedTypeBindings {
		if binding.EnneagramType != nil && *binding.EnneagramType == 6 {
			t.Fatalf("unrequested current-card type leaked into bindings: %+v", resolved)
		}
	}
}

func TestResolveBindingRowsEmptyRequestedTypesPreservesCurrentCardShape(t *testing.T) {
	rows := []bindingRow{
		{Layer: LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", LibraryStatus: "enabled", ReleaseID: 100, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(6), LibraryID: 16, LibraryKey: "enneagram-type-06", LibraryStatus: "enabled", ReleaseID: 106, ReleaseStatus: "active"},
	}

	resolved := resolveBindingRows(6, []int{0, 10, -1}, rows)
	if resolved.EnneagramType == nil || *resolved.EnneagramType.EnneagramType != 6 {
		t.Fatalf("current-card binding missing: %+v", resolved)
	}
	if len(resolved.RequestedTypeBindings) != 0 {
		t.Fatalf("empty normalized request created requested bindings: %+v", resolved.RequestedTypeBindings)
	}
}

func TestResolveConversationRequestedTypesUsesSingleRepeatableReadSnapshot(t *testing.T) {
	state := &resolverDBState{}
	driverName := fmt.Sprintf("appknowledge_resolver_%d", atomic.AddUint64(&resolverDriverSequence, 1))
	sql.Register(driverName, resolverTestDriver{state: state})
	database, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	resolved, err := NewResolver(database).ResolveConversation(context.Background(), 7, 8, 55, []int{4, 1, 4, 2, 0, 10})
	if err != nil {
		t.Fatal(err)
	}
	if state.beginCount != 1 || state.commitCount != 1 || state.rollbackCount != 0 || state.queryCount != 2 {
		t.Fatalf("snapshot begin/commit/rollback/queries = %d/%d/%d/%d", state.beginCount, state.commitCount, state.rollbackCount, state.queryCount)
	}
	if state.beginOptions.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) || !state.beginOptions.ReadOnly {
		t.Fatalf("transaction options = %+v", state.beginOptions)
	}
	if !reflect.DeepEqual(state.bindingArgs, []int64{1, 2, 4}) {
		t.Fatalf("binding args = %v, want normalized [1 2 4]", state.bindingArgs)
	}
	if strings.Contains(state.bindingQuery, "enneagram_type=$1") {
		t.Fatalf("explicit request unexpectedly used current-card-only query: %s", state.bindingQuery)
	}
	got := make([]int, 0, len(resolved.RequestedTypeBindings))
	for _, binding := range resolved.RequestedTypeBindings {
		got = append(got, *binding.EnneagramType)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 4}) {
		t.Fatalf("resolved requested bindings = %v", got)
	}
}

var resolverDriverSequence uint64

type resolverDBState struct {
	beginCount    int
	commitCount   int
	rollbackCount int
	queryCount    int
	beginOptions  driver.TxOptions
	bindingQuery  string
	bindingArgs   []int64
}

type resolverTestDriver struct{ state *resolverDBState }

func (d resolverTestDriver) Open(string) (driver.Conn, error) {
	return &resolverTestConn{state: d.state}, nil
}

type resolverTestConn struct{ state *resolverDBState }

func (c *resolverTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *resolverTestConn) Close() error                        { return nil }
func (c *resolverTestConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *resolverTestConn) BeginTx(_ context.Context, options driver.TxOptions) (driver.Tx, error) {
	c.state.beginCount++
	c.state.beginOptions = options
	return resolverTestTx{state: c.state}, nil
}
func (c *resolverTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.queryCount++
	if strings.Contains(query, "FROM app_chat_sessions") {
		return &resolverTestRows{
			columns: []string{"id", "enneagram", "revision"},
			values:  [][]driver.Value{{int64(55), int64(6), int64(12)}},
		}, nil
	}
	c.state.bindingQuery = query
	for _, arg := range args {
		value, ok := arg.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("binding arg %T is not int64", arg.Value)
		}
		c.state.bindingArgs = append(c.state.bindingArgs, value)
	}
	return &resolverTestRows{
		columns: []string{"layer_kind", "enneagram_type", "id", "key", "status", "release_id", "release_status"},
		values: [][]driver.Value{
			{"enneagram_type", int64(4), int64(14), "enneagram-type-04", "enabled", int64(104), "active"},
			{"theory", nil, int64(10), "enneagram-core", "enabled", int64(100), "active"},
			{"enneagram_type", int64(1), int64(11), "enneagram-type-01", "enabled", int64(101), "active"},
			{"enneagram_type", int64(2), int64(12), "enneagram-type-02", "enabled", int64(102), "active"},
		},
	}, nil
}

type resolverTestTx struct{ state *resolverDBState }

func (tx resolverTestTx) Commit() error   { tx.state.commitCount++; return nil }
func (tx resolverTestTx) Rollback() error { tx.state.rollbackCount++; return nil }

type resolverTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *resolverTestRows) Columns() []string { return r.columns }
func (r *resolverTestRows) Close() error      { return nil }
func (r *resolverTestRows) Next(destination []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(destination, r.values[r.index])
	r.index++
	return nil
}

func intPointer(value int) *int { return &value }
