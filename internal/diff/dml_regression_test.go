package diff

import (
	"encoding/json"
	"strings"
	"testing"

	"mysql-monitor/internal/model"
)

func TestDiffDataCompositeKeyDoesNotCollide(t *testing.T) {
	from := &model.DatabaseData{Tables: map[string]*model.TableData{
		"items": {PrimaryKeyColumns: []string{"a", "b"}, ColumnNames: []string{"a", "b"}, Rows: []map[string]interface{}{
			{"a": "x__PK_SEP__y", "b": "z"},
		}},
	}}
	to := &model.DatabaseData{Tables: map[string]*model.TableData{
		"items": {PrimaryKeyColumns: []string{"a", "b"}, ColumnNames: []string{"a", "b"}, Rows: []map[string]interface{}{
			{"a": "x", "b": "y__PK_SEP__z"},
		}},
	}}

	_, summary := DiffData(from, to)
	if summary.InsertCount != 1 || summary.DeleteCount != 1 || summary.UpdateCount != 0 {
		t.Fatalf("unexpected diff: %+v", summary)
	}
}

func TestDiffDataKeepsDuplicateRowsWithoutPrimaryKey(t *testing.T) {
	row := map[string]interface{}{"value": "same"}
	from := &model.DatabaseData{Tables: map[string]*model.TableData{
		"items": {ColumnNames: []string{"value"}, Rows: []map[string]interface{}{row, row}},
	}}
	to := &model.DatabaseData{Tables: map[string]*model.TableData{
		"items": {ColumnNames: []string{"value"}, Rows: []map[string]interface{}{row}},
	}}

	diffs, summary := DiffData(from, to)
	if summary.DeleteCount != 1 || summary.InsertCount != 0 || len(diffs) != 1 {
		t.Fatalf("unexpected diff: %+v", summary)
	}
	if got := diffs[0].DeleteStatements[0]; !strings.HasSuffix(got, "LIMIT 1;") {
		t.Fatalf("unkeyed deletion must affect one row: %s", got)
	}
}

func TestDiffDataDistinguishesValueTypesAndPreservesLargeKey(t *testing.T) {
	key := json.Number("9007199254740993")
	from := &model.DatabaseData{Tables: map[string]*model.TableData{
		"items": {PrimaryKeyColumns: []string{"id"}, ColumnNames: []string{"id", "value"}, Rows: []map[string]interface{}{
			{"id": key, "value": "1"},
		}},
	}}
	to := &model.DatabaseData{Tables: map[string]*model.TableData{
		"items": {PrimaryKeyColumns: []string{"id"}, ColumnNames: []string{"id", "value"}, Rows: []map[string]interface{}{
			{"id": key, "value": json.Number("1")},
		}},
	}}

	diffs, summary := DiffData(from, to)
	if summary.UpdateCount != 1 || len(diffs) != 1 {
		t.Fatalf("unexpected diff: %+v", summary)
	}
	if got := diffs[0].UpdateStatements[0]; !strings.Contains(got, "`id` = 9007199254740993") {
		t.Fatalf("large key lost precision: %s", got)
	}
}

func TestGenerateDiffResultSkipsMissingSchemaOnlyData(t *testing.T) {
	from := &model.Snapshot{
		Schema: &model.DatabaseSchema{Tables: map[string]*model.TableSchema{"items": {Name: "items"}}},
		Data: &model.DatabaseData{Tables: map[string]*model.TableData{
			"items": {PrimaryKeyColumns: []string{"id"}, ColumnNames: []string{"id"}, Rows: []map[string]interface{}{{"id": 1}}},
		}},
	}
	to := &model.Snapshot{
		Schema: &model.DatabaseSchema{Tables: map[string]*model.TableSchema{"items": {Name: "items"}}},
		Data:   &model.DatabaseData{Tables: map[string]*model.TableData{}},
	}

	result := GenerateDiffResult("conn", "from", "to", from, to)
	if result.Summary.DML.DeleteCount != 0 || len(result.TableDMLDiffs) != 0 {
		t.Fatalf("schema-only snapshot produced DML: %+v", result.Summary.DML)
	}
}

func TestGenerateDiffResultDoesNotDeleteRowsAfterDroppingTable(t *testing.T) {
	from := &model.Snapshot{
		Schema: &model.DatabaseSchema{Tables: map[string]*model.TableSchema{"items": {Name: "items"}}},
		Data: &model.DatabaseData{Tables: map[string]*model.TableData{
			"items": {PrimaryKeyColumns: []string{"id"}, ColumnNames: []string{"id"}, Rows: []map[string]interface{}{{"id": 1}}},
		}},
	}
	to := &model.Snapshot{
		Schema: &model.DatabaseSchema{Tables: map[string]*model.TableSchema{}},
		Data:   &model.DatabaseData{Tables: map[string]*model.TableData{}},
	}

	result := GenerateDiffResult("conn", "from", "to", from, to)
	if result.Summary.DDL.DroppedTables != 1 || result.Summary.DML.DeleteCount != 0 {
		t.Fatalf("unexpected diff: %+v", result.Summary)
	}
	if strings.Contains(result.FullScript, "DELETE FROM `items`") {
		t.Fatalf("script deletes from dropped table: %s", result.FullScript)
	}
}
